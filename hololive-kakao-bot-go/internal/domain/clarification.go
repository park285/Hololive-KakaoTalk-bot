package domain

type ClarificationResponse struct {
	Message   string `json:"message"`
	Candidate string `json:"candidate,omitempty"`
}

type Clarification struct {
	IsHololiveRelated bool   `json:"is_hololive_related"`
	Message           string `json:"message"`
	Candidate         string `json:"candidate,omitempty"`
}
