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
	Status     string `json:"status,omitempty"`
	CommitHash string `json:"commit_hash,omitempty"`
	Error      string `json:"error,omitempty"`
}

func (s *Slack) NotifyTaskFinished(ctx context.Context, batchID, runID, taskRef, status, commitHash string) error {
	return s.post(ctx, slackPayload{
		Text:       fmt.Sprintf("Task %s finished with status %s", taskRef, status),
		Type:       "task_finished",
		BatchID:    batchID,
		RunID:      runID,
		TaskRef:    taskRef,
		Status:     status,
		CommitHash: strings.TrimSpace(commitHash),
	})
}

func (s *Slack) NotifyBatchIdle(ctx context.Context, batchID string) error {
	return s.post(ctx, slackPayload{
		Text:    fmt.Sprintf("Batch %s is idle", batchID),
		Type:    "batch_idle",
		BatchID: batchID,
	})
}

func (s *Slack) NotifyError(ctx context.Context, batchID, runID, taskRef string, err error) error {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	return s.post(ctx, slackPayload{
		Text:    fmt.Sprintf("Error in task %s: %s", taskRef, msg),
		Type:    "error",
		BatchID: batchID,
		RunID:   runID,
		TaskRef: taskRef,
		Error:   msg,
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
