package store

import (
	"context"
	"time"

	"basegraph.co/relay/core/db/sqlc"
	"basegraph.co/relay/internal/model"
	"github.com/jackc/pgx/v5/pgtype"
)

type llmEvalStore struct {
	queries *sqlc.Queries
}

func newLLMEvalStore(queries *sqlc.Queries) LLMEvalStore {
	return &llmEvalStore{queries: queries}
}

func (s *llmEvalStore) Create(ctx context.Context, eval *model.LLMEval) (*model.LLMEval, error) {
	var latencyMs, promptTokens, completionTokens *int32
	if eval.LatencyMs != nil {
		v := int32(*eval.LatencyMs)
		latencyMs = &v
	}
	if eval.PromptTokens != nil {
		v := int32(*eval.PromptTokens)
		promptTokens = &v
	}
	if eval.CompletionTokens != nil {
		v := int32(*eval.CompletionTokens)
		completionTokens = &v
	}

	row, err := s.queries.InsertLLMEval(ctx, sqlc.InsertLLMEvalParams{
		ID:               eval.ID,
		WorkspaceID:      eval.WorkspaceID,
		IssueID:          eval.IssueID,
		Stage:            eval.Stage,
		InputText:        eval.InputText,
		OutputJson:       eval.OutputJSON,
		Model:            eval.Model,
		Temperature:      eval.Temperature,
		PromptVersion:    eval.PromptVersion,
		LatencyMs:        latencyMs,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
	})
	if err != nil {
		return nil, err
	}
	return toLLMEvalModel(row), nil
}

func (s *llmEvalStore) GetByID(ctx context.Context, id int64) (*model.LLMEval, error) {
	row, err := s.queries.GetLLMEvalByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return toLLMEvalModel(row), nil
}

func (s *llmEvalStore) ListByIssue(ctx context.Context, issueID int64) ([]model.LLMEval, error) {
	rows, err := s.queries.ListLLMEvalsByIssue(ctx, &issueID)
	if err != nil {
		return nil, err
	}
	return toLLMEvalModels(rows), nil
}

func (s *llmEvalStore) ListByStage(ctx context.Context, stage string, limit int32) ([]model.LLMEval, error) {
	rows, err := s.queries.ListLLMEvalsByStage(ctx, sqlc.ListLLMEvalsByStageParams{
		Stage: stage,
		Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	return toLLMEvalModels(rows), nil
}

func (s *llmEvalStore) ListUnrated(ctx context.Context, stage string, limit int32) ([]model.LLMEval, error) {
	rows, err := s.queries.ListUnratedLLMEvals(ctx, sqlc.ListUnratedLLMEvalsParams{
		Stage: stage,
		Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	return toLLMEvalModels(rows), nil
}

func (s *llmEvalStore) Rate(ctx context.Context, id int64, rating int, notes string, ratedByUserID int64) error {
	r := int32(rating)
	return s.queries.RateLLMEval(ctx, sqlc.RateLLMEvalParams{
		ID:            id,
		Rating:        &r,
		RatingNotes:   &notes,
		RatedByUserID: &ratedByUserID,
	})
}

func (s *llmEvalStore) SetExpected(ctx context.Context, id int64, expectedJSON []byte, evalScore float64) error {
	return s.queries.SetLLMEvalExpected(ctx, sqlc.SetLLMEvalExpectedParams{
		ID:           id,
		ExpectedJson: expectedJSON,
		EvalScore:    &evalScore,
	})
}

func (s *llmEvalStore) GetStats(ctx context.Context, stage string, since time.Time) (*model.LLMEvalStats, error) {
	row, err := s.queries.GetEvalStats(ctx, sqlc.GetEvalStatsParams{
		Stage: stage,
		CreatedAt: pgtype.Timestamptz{
			Time:  since,
			Valid: true,
		},
	})
	if err != nil {
		return nil, err
	}
	return &model.LLMEvalStats{
		Stage:        row.Stage,
		Total:        row.Total,
		Rated:        row.Rated,
		AvgRating:    row.AvgRating,
		AvgEvalScore: row.AvgEvalScore,
		AvgLatencyMs: row.AvgLatencyMs,
	}, nil
}

func toLLMEvalModel(row sqlc.LlmEval) *model.LLMEval {
	var latencyMs, promptTokens, completionTokens *int
	if row.LatencyMs != nil {
		v := int(*row.LatencyMs)
		latencyMs = &v
	}
	if row.PromptTokens != nil {
		v := int(*row.PromptTokens)
		promptTokens = &v
	}
	if row.CompletionTokens != nil {
		v := int(*row.CompletionTokens)
		completionTokens = &v
	}

	var rating *int
	if row.Rating != nil {
		v := int(*row.Rating)
		rating = &v
	}

	var ratedAt *time.Time
	if row.RatedAt.Valid {
		ratedAt = &row.RatedAt.Time
	}

	return &model.LLMEval{
		ID:               row.ID,
		WorkspaceID:      row.WorkspaceID,
		IssueID:          row.IssueID,
		Stage:            row.Stage,
		InputText:        row.InputText,
		OutputJSON:       row.OutputJson,
		Model:            row.Model,
		Temperature:      row.Temperature,
		PromptVersion:    row.PromptVersion,
		LatencyMs:        latencyMs,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		Rating:           rating,
		RatingNotes:      row.RatingNotes,
		RatedByUserID:    row.RatedByUserID,
		RatedAt:          ratedAt,
		ExpectedJSON:     row.ExpectedJson,
		EvalScore:        row.EvalScore,
		CreatedAt:        row.CreatedAt.Time,
	}
}

func toLLMEvalModels(rows []sqlc.LlmEval) []model.LLMEval {
	models := make([]model.LLMEval, len(rows))
	for i, row := range rows {
		models[i] = *toLLMEvalModel(row)
	}
	return models
}
