package store

import (
	"context"
	"encoding/json"
	"errors"

	"basegraph.app/relay/core/db/sqlc"
	"basegraph.app/relay/internal/model"
	"github.com/jackc/pgx/v5"
)

type issueStore struct {
	queries *sqlc.Queries
}

func newIssueStore(queries *sqlc.Queries) IssueStore {
	return &issueStore{queries: queries}
}

func (s *issueStore) Upsert(ctx context.Context, issue *model.Issue) (*model.Issue, error) {
	codeFindingsJSON, err := json.Marshal(issue.CodeFindings)
	if err != nil {
		return nil, err
	}
	learningsJSON, err := json.Marshal(issue.Learnings)
	if err != nil {
		return nil, err
	}
	discussionsJSON, err := json.Marshal(issue.Discussions)
	if err != nil {
		return nil, err
	}

	row, err := s.queries.UpsertIssue(ctx, sqlc.UpsertIssueParams{
		ID:              issue.ID,
		IntegrationID:   issue.IntegrationID,
		ExternalIssueID: issue.ExternalIssueID,
		Title:           issue.Title,
		Description:     issue.Description,
		Labels:          issue.Labels,
		Members:         issue.Members,
		Assignees:       issue.Assignees,
		Reporter:        issue.Reporter,
		Keywords:        issue.Keywords,
		CodeFindings:    codeFindingsJSON,
		Learnings:       learningsJSON,
		Discussions:     discussionsJSON,
		Spec:            issue.Spec,
	})
	if err != nil {
		return nil, err
	}
	return toIssueModel(row)
}

func (s *issueStore) GetByID(ctx context.Context, id int64) (*model.Issue, error) {
	row, err := s.queries.GetIssue(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toIssueModel(row)
}

func (s *issueStore) GetByIntegrationAndExternalID(ctx context.Context, integrationID int64, externalIssueID string) (*model.Issue, error) {
	row, err := s.queries.GetIssueByIntegrationAndExternalID(ctx, sqlc.GetIssueByIntegrationAndExternalIDParams{
		IntegrationID:   integrationID,
		ExternalIssueID: externalIssueID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toIssueModel(row)
}

func toIssueModel(row sqlc.Issue) (*model.Issue, error) {
	var codeFindings []model.CodeFinding
	if len(row.CodeFindings) > 0 {
		if err := json.Unmarshal(row.CodeFindings, &codeFindings); err != nil {
			return nil, err
		}
	}

	var learnings []model.Learning
	if len(row.Learnings) > 0 {
		if err := json.Unmarshal(row.Learnings, &learnings); err != nil {
			return nil, err
		}
	}

	var discussions []model.Discussion
	if len(row.Discussions) > 0 {
		if err := json.Unmarshal(row.Discussions, &discussions); err != nil {
			return nil, err
		}
	}

	return &model.Issue{
		ID:              row.ID,
		IntegrationID:   row.IntegrationID,
		ExternalIssueID: row.ExternalIssueID,
		Title:           row.Title,
		Description:     row.Description,
		Labels:          row.Labels,
		Members:         row.Members,
		Assignees:       row.Assignees,
		Reporter:        row.Reporter,
		Keywords:        row.Keywords,
		CodeFindings:    codeFindings,
		Learnings:       learnings,
		Discussions:     discussions,
		Spec:            row.Spec,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}, nil
}
