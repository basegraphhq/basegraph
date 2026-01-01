package model

import "time"

type LLMEvalStage string

const (
	LLMEvalStageKeywords      LLMEvalStage = "keywords"
	LLMEvalStagePlanner       LLMEvalStage = "planner"
	LLMEvalStageGapDetector   LLMEvalStage = "gap_detector"
	LLMEvalStageSpecGenerator LLMEvalStage = "spec_generator"
)

type LLMEval struct {
	ID          int64  `json:"id"`
	WorkspaceID *int64 `json:"workspace_id,omitempty"`
	IssueID     *int64 `json:"issue_id,omitempty"`

	Stage string `json:"stage"`

	InputText  string `json:"input_text"`
	OutputJSON []byte `json:"output_json"`

	Model         string   `json:"model"`
	Temperature   *float64 `json:"temperature,omitempty"`
	PromptVersion *string  `json:"prompt_version,omitempty"`

	LatencyMs        *int `json:"latency_ms,omitempty"`
	PromptTokens     *int `json:"prompt_tokens,omitempty"`
	CompletionTokens *int `json:"completion_tokens,omitempty"`

	Rating        *int       `json:"rating,omitempty"`
	RatingNotes   *string    `json:"rating_notes,omitempty"`
	RatedByUserID *int64     `json:"rated_by_user_id,omitempty"`
	RatedAt       *time.Time `json:"rated_at,omitempty"`

	ExpectedJSON []byte   `json:"expected_json,omitempty"`
	EvalScore    *float64 `json:"eval_score,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

type LLMEvalStats struct {
	Stage        string  `json:"stage"`
	Total        int64   `json:"total"`
	Rated        int64   `json:"rated"`
	AvgRating    float64 `json:"avg_rating"`
	AvgEvalScore float64 `json:"avg_eval_score"`
	AvgLatencyMs int32   `json:"avg_latency_ms"`
}
