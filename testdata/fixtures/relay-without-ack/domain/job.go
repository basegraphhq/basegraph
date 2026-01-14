package domain

// JobType enumerates the different retrieval or processing jobs the planner can schedule.
type JobType string

const (
	JobExtractKeywords   JobType = "extract_keywords"
	JobRetrieveCode      JobType = "retrieve_code"
	JobRetrieveLearnings JobType = "retrieve_learnings"
)

// Job represents a unit of work that the executor should perform.
type Job struct {
	Type    JobType         `json:"type"`
	Payload map[string]any  `json:"payload,omitempty"`
	Reason  string          `json:"reason,omitempty"`
	Depends []JobDependency `json:"depends,omitempty"`
}

// JobDependency describes prerequisites for executing a job.
type JobDependency struct {
	JobType JobType `json:"job_type"`
	Reason  string  `json:"reason,omitempty"`
}
