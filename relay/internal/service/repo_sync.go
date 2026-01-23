package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"basegraph.co/relay/common/id"
	"basegraph.co/relay/internal/model"
	"basegraph.co/relay/internal/queue"
	"basegraph.co/relay/internal/store"
)

type RepoSyncService interface {
	Enqueue(ctx context.Context, params RepoSyncParams) (*RepoSyncResult, error)
}

type RepoSyncParams struct {
	IntegrationID  int64
	ExternalRepoID string
	Ref            string
	After          string
	TraceID        *string
}

type RepoSyncResult struct {
	Enqueued bool
	Reason   string
	RunID    *int64
	RepoID   *int64
	Branch   string
}

type repoSyncService struct {
	integrations       store.IntegrationStore
	repos              store.RepoStore
	workspaceEventLogs store.WorkspaceEventLogStore
	producer           queue.Producer
}

func NewRepoSyncService(
	integrations store.IntegrationStore,
	repos store.RepoStore,
	workspaceEventLogs store.WorkspaceEventLogStore,
	producer queue.Producer,
) RepoSyncService {
	return &repoSyncService{
		integrations:       integrations,
		repos:              repos,
		workspaceEventLogs: workspaceEventLogs,
		producer:           producer,
	}
}

func (s *repoSyncService) Enqueue(ctx context.Context, params RepoSyncParams) (*RepoSyncResult, error) {
	if params.IntegrationID == 0 {
		return nil, fmt.Errorf("integration id is required")
	}
	if params.ExternalRepoID == "" {
		return nil, fmt.Errorf("external repo id is required")
	}

	branch := parseBranchRef(params.Ref)
	if branch == "" {
		return &RepoSyncResult{Enqueued: false, Reason: "unsupported_ref"}, nil
	}

	integration, err := s.integrations.GetByID(ctx, params.IntegrationID)
	if err != nil {
		return nil, fmt.Errorf("fetching integration: %w", err)
	}

	repo, err := s.repos.GetByExternalID(ctx, params.IntegrationID, params.ExternalRepoID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return &RepoSyncResult{Enqueued: false, Reason: "repo_not_found"}, nil
		}
		return nil, fmt.Errorf("fetching repository: %w", err)
	}

	if !repo.IsEnabled {
		return &RepoSyncResult{Enqueued: false, Reason: "repo_disabled"}, nil
	}

	if repo.DefaultBranch == nil || strings.TrimSpace(*repo.DefaultBranch) == "" {
		return &RepoSyncResult{Enqueued: false, Reason: "default_branch_missing"}, nil
	}

	if *repo.DefaultBranch != branch {
		return &RepoSyncResult{Enqueued: false, Reason: "non_default_branch", Branch: branch, RepoID: &repo.ID}, nil
	}

	metadata, err := json.Marshal(map[string]string{
		"ref":    params.Ref,
		"branch": branch,
		"after":  params.After,
	})
	if err != nil {
		return nil, fmt.Errorf("serializing metadata: %w", err)
	}

	runID := id.New()
	log := &model.WorkspaceEventLog{
		ID:             runID,
		WorkspaceID:    integration.WorkspaceID,
		OrganizationID: integration.OrganizationID,
		RepoID:         &repo.ID,
		EventType:      string(model.WorkspaceEventTypeRepoSync),
		Status:         string(model.WorkspaceEventStatusQueued),
		Metadata:       metadata,
	}

	if _, err := s.workspaceEventLogs.Create(ctx, log); err != nil {
		return nil, fmt.Errorf("creating repo sync log: %w", err)
	}

	if err := s.producer.Enqueue(ctx, queue.Task{
		TaskType:       queue.TaskTypeRepoSync,
		WorkspaceID:    &integration.WorkspaceID,
		OrganizationID: &integration.OrganizationID,
		RunID:          &runID,
		RepoID:         &repo.ID,
		Branch:         branch,
		Attempt:        1,
		TraceID:        params.TraceID,
	}); err != nil {
		errMsg := err.Error()
		log.Status = string(model.WorkspaceEventStatusFailed)
		log.Error = &errMsg
		finishedAt := time.Now().UTC()
		_, _ = s.workspaceEventLogs.Update(ctx, log, nil, &finishedAt)
		return nil, fmt.Errorf("enqueueing repo sync: %w", err)
	}

	return &RepoSyncResult{
		Enqueued: true,
		RunID:    &runID,
		RepoID:   &repo.ID,
		Branch:   branch,
	}, nil
}

func parseBranchRef(ref string) string {
	if after, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
		return after
	}
	return ""
}
