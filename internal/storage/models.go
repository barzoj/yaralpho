package storage

import "time"

// BatchStatus captures lifecycle states for a batch of work.
type BatchStatus string

const (
	// BatchStatusPending represents a batch that is created and awaiting scheduling.
	BatchStatusPending BatchStatus = "pending"
	BatchStatusCreated BatchStatus = "created"
	BatchStatusRunning BatchStatus = "running"
	BatchStatusIdle    BatchStatus = "idle"
	// BatchStatusInProgress mirrors pending->in_progress transitions for repository-aware scheduling.
	BatchStatusInProgress BatchStatus = "in_progress"
	// BatchStatusPaused prevents new items from starting while allowing in-flight work to finish.
	BatchStatusPaused      BatchStatus = "paused"
	BatchStatusDone        BatchStatus = "done"
	BatchStatusFailed      BatchStatus = "failed"
	BatchStatusBlockedAuth BatchStatus = "blocked_auth"
)

// ItemStatus captures lifecycle states for a single batch item.
type ItemStatus string

const (
	ItemStatusPending    ItemStatus = "pending"
	ItemStatusInProgress ItemStatus = "in_progress"
	ItemStatusDone       ItemStatus = "done"
	ItemStatusFailed     ItemStatus = "failed"
)

// TaskRunStatus represents execution states for a single task run.
type TaskRunStatus string

const (
	TaskRunStatusRunning   TaskRunStatus = "running"
	TaskRunStatusSucceeded TaskRunStatus = "succeeded"
	TaskRunStatusFailed    TaskRunStatus = "failed"
	TaskRunStatusStopped   TaskRunStatus = "stopped"
)

// Repository represents a source code repository whose batches and runs are scoped together.
type Repository struct {
	ID        string    `json:"repository_id" bson:"repository_id"`
	Name      string    `json:"name" bson:"name"`
	Path      string    `json:"path" bson:"path"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}

// BatchItem represents a single ordered item within a batch and its retry state.
type BatchItem struct {
	Input    string `json:"input" bson:"input"`
	Status   string `json:"status" bson:"status"`
	Attempts int    `json:"attempts" bson:"attempts"`
}

// Batch groups a set of input items processed together and tracked as a unit.
type Batch struct {
	ID           string      `json:"batch_id" bson:"batch_id"`
	RepositoryID string      `json:"repository_id" bson:"repository_id"`
	CreatedAt    time.Time   `json:"created_at" bson:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at" bson:"updated_at"`
	Items        []BatchItem `json:"items" bson:"items"`
	Status       BatchStatus `json:"status" bson:"status"`
	Summary      string      `json:"summary,omitempty" bson:"summary,omitempty"`
	SessionName  string      `json:"session_name,omitempty" bson:"session_name,omitempty"`
}

// TaskRun represents a single execution attempt for a task within a batch.
type TaskRun struct {
	ID           string        `json:"run_id" bson:"run_id"`
	BatchID      string        `json:"batch_id" bson:"batch_id"`
	RepositoryID string        `json:"repository_id" bson:"repository_id"`
	TaskRef      string        `json:"task_ref" bson:"task_ref"`
	EpicRef      string        `json:"epic_ref,omitempty" bson:"epic_ref,omitempty"`
	ParentRef    string        `json:"parent_ref,omitempty" bson:"parent_ref,omitempty"`
	SessionID    string        `json:"session_id" bson:"session_id"`
	StartedAt    time.Time     `json:"started_at" bson:"started_at"`
	FinishedAt   *time.Time    `json:"finished_at,omitempty" bson:"finished_at,omitempty"`
	Status       TaskRunStatus `json:"status" bson:"status"`
	Result       *RunResult    `json:"result,omitempty" bson:"result,omitempty"`
}

// TaskRunSummary represents a task run with aggregated metadata for listing.
type TaskRunSummary struct {
	TaskRun     `bson:",inline"`
	TotalEvents int64 `json:"total_events" bson:"total_events"`
}

// AgentStatus captures runtime availability of an agent.
type AgentStatus string

const (
	AgentStatusIdle AgentStatus = "idle"
	AgentStatusBusy AgentStatus = "busy"
)

// Agent represents a runtime worker that can execute tasks (codex or copilot).
type Agent struct {
	ID        string      `json:"agent_id" bson:"agent_id"`
	Name      string      `json:"name" bson:"name"`
	Type      string      `json:"type" bson:"type"`
	Status    AgentStatus `json:"status" bson:"status"`
	CreatedAt time.Time   `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time   `json:"updated_at" bson:"updated_at"`
}

// RunResult captures optional outputs from a completed task run.
type RunResult struct {
	CommitHash   string `json:"commit_hash,omitempty" bson:"commit_hash,omitempty"`
	FinalMessage string `json:"final_message,omitempty" bson:"final_message,omitempty"`
}

// SessionEvent stores raw Copilot session events for later analysis or replay.
type SessionEvent struct {
	BatchID    string         `json:"batch_id" bson:"batch_id"`
	RunID      string         `json:"run_id" bson:"run_id"`
	SessionID  string         `json:"session_id" bson:"session_id"`
	Event      map[string]any `json:"event" bson:"event"`
	IngestedAt time.Time      `json:"ingested_at" bson:"ingested_at"`
}

// BatchProgress summarizes task run counts for a batch.
// "Done" may be derived by callers as Succeeded + Stopped when needed.
type BatchProgress struct {
	Total     int `json:"total" bson:"total"`
	Pending   int `json:"pending" bson:"pending"`
	Running   int `json:"running" bson:"running"`
	Succeeded int `json:"succeeded" bson:"succeeded"`
	Failed    int `json:"failed" bson:"failed"`
	Stopped   int `json:"stopped" bson:"stopped"`
}
