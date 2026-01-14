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
	keywordsJSON, err := json.Marshal(issue.Keywords)
	if err != nil {
		return nil, err
	}
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
		ID:                issue.ID,
		IntegrationID:     issue.IntegrationID,
		ExternalProjectID: issue.ExternalProjectID,
		ExternalIssueID:   issue.ExternalIssueID,
		Provider:          string(issue.Provider),
		Title:             issue.Title,
		Description:       issue.Description,
		Labels:            issue.Labels,
		Members:           issue.Members,
		Assignees:         issue.Assignees,
		Reporter:          issue.Reporter,
		Keywords:          keywordsJSON,
		CodeFindings:      codeFindingsJSON,
		Learnings:         learningsJSON,
		Discussions:       discussionsJSON,
		Spec:              issue.Spec,
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

func (s *issueStore) QueueIfIdle(ctx context.Context, issueID int64) (bool, error) {
	_, err := s.queries.QueueIssueIfIdle(ctx, issueID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Issue was not idle (already queued or processing)
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *issueStore) ClaimQueued(ctx context.Context, issueID int64) (bool, *model.Issue, error) {
	row, err := s.queries.ClaimQueuedIssue(ctx, issueID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Issue was not queued (already claimed or idle)
			return false, nil, nil
		}
		return false, nil, err
	}
	issue, err := toIssueModel(row)
	if err != nil {
		return false, nil, err
	}
	return true, issue, nil
}

func (s *issueStore) SetIdle(ctx context.Context, issueID int64) error {
	rowsAffected, err := s.queries.SetIssueIdle(ctx, issueID)
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("issue was not in processing state")
	}
	return nil
}

func (s *issueStore) ResetQueuedToIdle(ctx context.Context, issueID int64) error {
	rowsAffected, err := s.queries.ResetIssueQueuedToIdle(ctx, issueID)
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("issue was not in queued state")
	}
	return nil
}

func (s *issueStore) GetByIDForUpdate(ctx context.Context, id int64) (*model.Issue, error) {
	row, err := s.queries.GetIssueForUpdate(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toIssueModel(row)
}

func (s *issueStore) UpdateCodeFindings(ctx context.Context, id int64, findings []model.CodeFinding) error {
	findingsJSON, err := json.Marshal(findings)
	if err != nil {
		return err
	}
	return s.queries.UpdateIssueCodeFindings(ctx, sqlc.UpdateIssueCodeFindingsParams{
		ID:           id,
		CodeFindings: findingsJSON,
	})
}

func (s *issueStore) UpdateSpec(ctx context.Context, id int64, spec *string) error {
	return s.queries.UpdateIssueSpec(ctx, sqlc.UpdateIssueSpecParams{
		ID:   id,
		Spec: spec,
	})
}

func (s *issueStore) UpdateSpecStatus(ctx context.Context, id int64, status model.SpecStatus) error {
	statusStr := string(status)
	return s.queries.UpdateIssueSpecStatus(ctx, sqlc.UpdateIssueSpecStatusParams{
		ID:         id,
		SpecStatus: &statusStr,
	})
}

func toIssueModel(row sqlc.Issue) (*model.Issue, error) {
	var keywords []model.Keyword
	if len(row.Keywords) > 0 {
		if err := json.Unmarshal(row.Keywords, &keywords); err != nil {
			return nil, err
		}
	}

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

	issue := &model.Issue{
		ID:                row.ID,
		IntegrationID:     row.IntegrationID,
		ExternalProjectID: row.ExternalProjectID,
		ExternalIssueID:   row.ExternalIssueID,
		Provider:          model.Provider(row.Provider),
		Title:             row.Title,
		Description:       row.Description,
		Labels:            row.Labels,
		Members:           row.Members,
		Assignees:         row.Assignees,
		Reporter:          row.Reporter,
		Keywords:          keywords,
		CodeFindings:      codeFindings,
		Learnings:         learnings,
		Discussions:       discussions,
		Spec:              row.Spec,
		ProcessingStatus:  model.ProcessingStatus(row.ProcessingStatus),
		CreatedAt:         row.CreatedAt.Time,
		UpdatedAt:         row.UpdatedAt.Time,
	}

	if row.SpecStatus != nil {
		status := model.SpecStatus(*row.SpecStatus)
		issue.SpecStatus = &status
	}

	if row.ProcessingStartedAt.Valid {
		t := row.ProcessingStartedAt.Time
		issue.ProcessingStartedAt = &t
	}
	if row.LastProcessedAt.Valid {
		t := row.LastProcessedAt.Time
		issue.LastProcessedAt = &t
	}

	return issue, nil
}
