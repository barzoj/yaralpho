package consumer

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgentStructuredResponseJSONTags(t *testing.T) {
	resp := AgentStructuredResponse{
		Status:             "success",
		SuccessSummary:     "all good",
		SuccessEvidence:    []string{"e1", "e2"},
		FailureSummary:     "failure summary",
		FailureReasons:     []string{"r1", "r2"},
		AssistantMessages:  []string{"msg1", "msg2"},
		AssistantReasoning: []string{"why1"},
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(data, &payload))

	expectedKeys := []string{
		"status",
		"success_summary",
		"success_evidence",
		"failure_summary",
		"failure_reasons",
		"assistant_messages",
		"assistant_reasoning",
	}

	for _, key := range expectedKeys {
		require.Contains(t, payload, key)
	}
}
