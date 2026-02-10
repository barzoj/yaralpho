package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/barzoj/yaralpho/internal/config"
	"go.uber.org/zap"
)

const defaultSlackTimeout = 5 * time.Second

// Slack posts notifications to a Slack webhook. When webhookURL is empty the
// constructor should return a Noop instead.
type Slack struct {
	webhookURL string
	client     *http.Client
	logger     *zap.Logger
}

// NewSlack constructs a Slack notifier using the webhook URL from config. An
// empty or missing webhook yields a Noop notifier. A nil config results in an
// error.
func NewSlack(cfg config.Config, logger *zap.Logger) (Notifier, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	url, _ := cfg.Get(config.SlackWebhookKey)
	url = strings.TrimSpace(url)
	if url == "" {
		return Noop{}, nil
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	return &Slack{
		webhookURL: url,
		client:     &http.Client{Timeout: defaultSlackTimeout},
		logger:     logger,
	}, nil
}

type slackPayload struct {
	Text       string `json:"text"`
	Type       string `json:"type"`
	BatchID    string `json:"batch_id"`
	RunID      string `json:"run_id,omitempty"`
	TaskRef    string `json:"task_ref,omitempty"`
	ParentTask string `json:"parent_task_ref,omitempty"`
	Status     string `json:"status,omitempty"`
	Details    string `json:"details,omitempty"`
	Attempt    int    `json:"attempt,omitempty"`
	MaxAttempt int    `json:"max_attempt,omitempty"`
	CommitHash string `json:"commit_hash,omitempty"`
	Error      string `json:"error,omitempty"`
}

func (s *Slack) NotifyEvent(ctx context.Context, event Event) error {
	text := s.textForEvent(event)
	return s.post(ctx, slackPayload{
		Text:       text,
		Type:       event.Type,
		BatchID:    event.BatchID,
		RunID:      event.RunID,
		TaskRef:    event.TaskRef,
		ParentTask: event.ParentTaskRef,
		Status:     event.Status,
		Details:    event.Details,
		Attempt:    event.Attempt,
		MaxAttempt: event.MaxAttempts,
		CommitHash: strings.TrimSpace(event.CommitHash),
	})
}

func (s *Slack) NotifyTaskFinished(ctx context.Context, batchID, runID, taskRef, status, commitHash string) error {
	return s.NotifyEvent(ctx, Event{
		Type:       "task_finished",
		BatchID:    batchID,
		RunID:      runID,
		TaskRef:    taskRef,
		Status:     status,
		CommitHash: commitHash,
	})
}

func (s *Slack) NotifyBatchIdle(ctx context.Context, batchID string) error {
	return s.NotifyEvent(ctx, Event{
		Type:    "batch_idle",
		BatchID: batchID,
		Status:  "idle",
	})
}

func (s *Slack) NotifyError(ctx context.Context, batchID, runID, taskRef string, err error) error {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	return s.NotifyEvent(ctx, Event{
		Type:    "error",
		BatchID: batchID,
		RunID:   runID,
		TaskRef: taskRef,
		Status:  "error",
		Details: msg,
	})
}

func (s *Slack) post(ctx context.Context, payload slackPayload) error {
	if s == nil {
		return errors.New("slack notifier is nil")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.webhookURL, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		s.logger.Error("slack post failed", zap.Error(err))
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("slack webhook status %d", resp.StatusCode)
		s.logger.Error("slack post non-2xx", zap.Int("status", resp.StatusCode))
		return err
	}

	return nil
}

func (s *Slack) textForEvent(event Event) string {
	parts := []string{}
	if event.Type != "" {
		parts = append(parts, fmt.Sprintf("[%s]", event.Type))
	}
	if event.BatchID != "" {
		parts = append(parts, fmt.Sprintf("batch=%s", event.BatchID))
	}
	if event.TaskRef != "" {
		parts = append(parts, fmt.Sprintf("task=%s", event.TaskRef))
	}
	if event.ParentTaskRef != "" {
		parts = append(parts, fmt.Sprintf("parent=%s", event.ParentTaskRef))
	}
	if event.Status != "" {
		parts = append(parts, fmt.Sprintf("status=%s", event.Status))
	}
	if event.Attempt > 0 {
		segment := fmt.Sprintf("attempt=%d", event.Attempt)
		if event.MaxAttempts > 0 {
			segment = fmt.Sprintf("attempt=%d/%d", event.Attempt, event.MaxAttempts)
		}
		parts = append(parts, segment)
	}
	if event.Details != "" {
		parts = append(parts, fmt.Sprintf("details=%s", event.Details))
	}
	if event.CommitHash != "" {
		parts = append(parts, fmt.Sprintf("commit=%s", strings.TrimSpace(event.CommitHash)))
	}
	return strings.Join(parts, " | ")
}
