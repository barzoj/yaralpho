package consumer

import (
	"context"
	"fmt"
	"time"

	"github.com/barzoj/yaralpho/internal/copilot"
	"github.com/barzoj/yaralpho/internal/notify"
	"github.com/barzoj/yaralpho/internal/storage"
	"go.uber.org/zap"
)

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

			err := st.InsertSessionEvent(ctx, &storage.SessionEvent{
				BatchID:    batch.ID,
				RunID:      run.ID,
				SessionID:  sessionID,
				Event:      evt,
				IngestedAt: now(),
			})
			if err != nil {
				status = storage.TaskRunStatusFailed
				finalErr = err
				logger.Error("insert session event", zap.Error(err), zap.String("run_id", run.ID))
				break eventLoop
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
