package consumer

// AgentStructuredResponse captures structured verification output from the agent.
// It mirrors the JSON contract expected from the verification prompt.
type AgentStructuredResponse struct {
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Details string `json:"details"`
}
