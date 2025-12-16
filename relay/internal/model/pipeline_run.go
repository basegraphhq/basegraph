package model

import "time"

type PipelineRun struct {
	ID         int64      `json:"id"`
	EventLogID int64      `json:"event_log_id"`
	Attempt    int32      `json:"attempt"`
	Status     string     `json:"status"`
	Error      *string    `json:"error,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}
