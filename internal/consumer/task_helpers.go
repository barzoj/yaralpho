package consumer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/barzoj/yaralpho/internal/bus"
	"github.com/barzoj/yaralpho/internal/copilot"
	"github.com/barzoj/yaralpho/internal/notify"
	"github.com/barzoj/yaralpho/internal/storage"
	"go.uber.org/zap"
)

var sessionEventBus bus.Bus

func setSessionEventBus(b bus.Bus) {
	sessionEventBus = b
}

func executeTask(
	ctx context.Context,
	cp copilot.Client,
	st storage.Storage,
	nt notify.Notifier,
	logger *zap.Logger,
	repoPath string,
	newRunID func() string,
	now func() time.Time,
	batch *storage.Batch,
	runRef,
	epicRef,
	prompt string,
) (storage.TaskRunStatus, error) {
	runID := newRunID()

	sessionID, events, stop, err := cp.StartSession(ctx, prompt, repoPath)
	if err != nil {
		finished := now()
		run := storage.TaskRun{
			ID:         runID,
			BatchID:    batch.ID,
			TaskRef:    runRef,
			EpicRef:    epicRef,
			SessionID:  "",
			StartedAt:  now(),
			FinishedAt: &finished,
			Status:     storage.TaskRunStatusFailed,
		}
		if createErr := st.CreateTaskRun(ctx, &run); createErr != nil {
			logger.Error("record failed run", zap.Error(createErr), zap.String("run_id", run.ID))
		}

		setBatchStatus(ctx, st, logger, batch, storage.BatchStatusFailed)
		_ = nt.NotifyError(ctx, batch.ID, runID, runRef, err)
		return storage.TaskRunStatusFailed, fmt.Errorf("start copilot session: %w", err)
	}
	defer stop()

	run := storage.TaskRun{
		ID:        runID,
		BatchID:   batch.ID,
		TaskRef:   runRef,
		EpicRef:   epicRef,
		SessionID: sessionID,
		StartedAt: now(),
		Status:    storage.TaskRunStatusRunning,
	}

	if err := st.CreateTaskRun(ctx, &run); err != nil {
		setBatchStatus(ctx, st, logger, batch, storage.BatchStatusFailed)
		_ = nt.NotifyError(ctx, batch.ID, runID, runRef, err)
		return storage.TaskRunStatusFailed, fmt.Errorf("create task run: %w", err)
	}

	status := storage.TaskRunStatusSucceeded
	var finalErr error

eventLoop:
	for {
		select {
		case <-ctx.Done():
			status = storage.TaskRunStatusStopped
			finalErr = ctx.Err()
			break eventLoop
		case evt, ok := <-events:
			if !ok {
				break eventLoop
			}

			sessionEvent := &storage.SessionEvent{
				BatchID:    batch.ID,
				RunID:      run.ID,
				SessionID:  sessionID,
				Event:      evt,
				IngestedAt: now(),
			}

			if err := st.InsertSessionEvent(ctx, sessionEvent); err != nil {
				status = storage.TaskRunStatusFailed
				finalErr = err
				logger.Error("insert session event", zap.Error(err), zap.String("run_id", run.ID))
				break eventLoop
			}

			if sessionEventBus != nil {
				if err := sessionEventBus.Publish(ctx, sessionID, *sessionEvent); err != nil {
					logger.Warn("publish session event", zap.Error(err), zap.String("run_id", run.ID), zap.String("session_id", sessionID), zap.String("batch_id", batch.ID))
					_ = nt.NotifyError(ctx, batch.ID, run.ID, runRef, fmt.Errorf("publish session event: %w", err))
				}
			}

			if isSessionIdleEvent(evt) {
				logger.Debug("copilot session idle event received; stopping run", zap.String("run_id", run.ID), zap.String("session_id", sessionID))
				break eventLoop
			}
		}
	}

	finished := now()
	run.Status = status
	run.FinishedAt = &finished

	if err := st.UpdateTaskRun(ctx, &run); err != nil {
		logger.Error("update task run", zap.Error(err), zap.String("run_id", run.ID))
	}

	switch status {
	case storage.TaskRunStatusSucceeded:
		_ = nt.NotifyTaskFinished(ctx, batch.ID, run.ID, run.TaskRef, string(run.Status), "")
	case storage.TaskRunStatusStopped:
		_ = nt.NotifyBatchIdle(ctx, batch.ID)
		setBatchStatus(ctx, st, logger, batch, storage.BatchStatusIdle)
	default:
		if finalErr == nil {
			finalErr = fmt.Errorf("task run failed")
		}
		_ = nt.NotifyError(ctx, batch.ID, run.ID, run.TaskRef, finalErr)
		setBatchStatus(ctx, st, logger, batch, storage.BatchStatusFailed)
	}

	return status, finalErr
}

func executeTaskWithStructuredOutput(
	ctx context.Context,
	cp copilot.Client,
	st storage.Storage,
	nt notify.Notifier,
	logger *zap.Logger,
	repoPath string,
	newRunID func() string,
	now func() time.Time,
	batch *storage.Batch,
	runRef,
	epicRef,
	prompt string,
) (storage.TaskRunStatus, string, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	// Capture the run ID up front so we can retrieve the persisted run and its session events after execution.
	generatedRunID := newRunID()
	status, err := executeTask(ctx, cp, st, nt, logger, repoPath, func() string { return generatedRunID }, now, batch, runRef, epicRef, prompt)

	structuredOutput := ""
	if st == nil || generatedRunID == "" {
		return status, structuredOutput, err
	}

	run, getRunErr := st.GetTaskRun(ctx, generatedRunID)
	if getRunErr != nil {
		logger.Debug("fetch task run for structured output", zap.Error(getRunErr), zap.String("run_id", generatedRunID))
		return status, structuredOutput, err
	}

	if run.SessionID == "" {
		return status, structuredOutput, err
	}

	events, listErr := st.ListSessionEvents(ctx, run.SessionID)
	if listErr != nil {
		logger.Debug("list session events for structured output", zap.Error(listErr), zap.String("session_id", run.SessionID), zap.String("run_id", generatedRunID))
		return status, structuredOutput, err
	}

	structuredOutput = latestAssistantMessageContent(events)
	return status, structuredOutput, err
}

// executeTaskWithAssistantMessages wraps executeTask to return all assistant.message contents
// concatenated with newlines in the order they were received.
func executeTaskWithAssistantMessages(
	ctx context.Context,
	cp copilot.Client,
	st storage.Storage,
	nt notify.Notifier,
	logger *zap.Logger,
	repoPath string,
	newRunID func() string,
	now func() time.Time,
	batch *storage.Batch,
	runRef,
	epicRef,
	prompt string,
) (storage.TaskRunStatus, string, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	generatedRunID := newRunID()
	status, err := executeTask(ctx, cp, st, nt, logger, repoPath, func() string { return generatedRunID }, now, batch, runRef, epicRef, prompt)

	if st == nil || generatedRunID == "" {
		return status, "", err
	}

	run, getRunErr := st.GetTaskRun(ctx, generatedRunID)
	if getRunErr != nil {
		logger.Debug("fetch task run for assistant messages", zap.Error(getRunErr), zap.String("run_id", generatedRunID))
		return status, "", err
	}

	if run.SessionID == "" {
		return status, "", err
	}

	events, listErr := st.ListSessionEvents(ctx, run.SessionID)
	if listErr != nil {
		logger.Debug("list session events for assistant messages", zap.Error(listErr), zap.String("session_id", run.SessionID), zap.String("run_id", generatedRunID))
		return status, "", err
	}

	messages := assistantMessageContents(events)
	return status, strings.Join(messages, "\n"), err
}

func setBatchStatus(ctx context.Context, st storage.Storage, logger *zap.Logger, batch *storage.Batch, status storage.BatchStatus) {
	if batch == nil {
		return
	}
	if batch.Status == status {
		return
	}
	batch.Status = status
	if err := st.UpdateBatch(ctx, batch); err != nil {
		logger.Error("update batch status", zap.Error(err), zap.String("batch_id", batch.ID))
	}
}

func latestAssistantMessageContent(events []storage.SessionEvent) string {
	messages := assistantMessageContents(events)
	if len(messages) == 0 {
		return ""
	}
	return messages[len(messages)-1]
}

func assistantMessageContents(events []storage.SessionEvent) []string {
	msgs := make([]string, 0)
	for _, evt := range events {
		eventType, ok := evt.Event["type"].(string)
		if !ok || eventType != "assistant.message" {
			continue
		}

		data, ok := evt.Event["data"].(map[string]any)
		if !ok {
			continue
		}

		content, ok := data["content"].(string)
		if ok {
			msgs = append(msgs, content)
		}
	}

	return msgs
}
