package consumer

// AgentStructuredResponse captures structured verification output from the agent.
type AgentStructuredResponse struct {
	Status             string   `json:"status"`
	SuccessSummary     string   `json:"success_summary,omitempty"`
	SuccessEvidence    []string `json:"success_evidence,omitempty"`
	FailureSummary     string   `json:"failure_summary,omitempty"`
	FailureReasons     []string `json:"failure_reasons,omitempty"`
	AssistantMessages  []string `json:"assistant_messages,omitempty"`
	AssistantReasoning []string `json:"assistant_reasoning,omitempty"`
}
