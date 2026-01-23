package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"basegraph.co/relay/common/id"
	"basegraph.co/relay/internal/model"
	"basegraph.co/relay/internal/queue"
	"basegraph.co/relay/internal/store"
	"github.com/redis/go-redis/v9"
)

type WorkspaceSetupService interface {
	Enqueue(ctx context.Context, workspaceID int64) (*WorkspaceSetupResult, error)
}

type WorkspaceSetupResult struct {
	RunID int64
}

type workspaceSetupService struct {
	workspaces          store.WorkspaceStore
	workspaceEventLogs  store.WorkspaceEventLogStore
	producer            queue.Producer
	redis               *redis.Client
	redisGroup          string
}

func NewWorkspaceSetupService(
	workspaces store.WorkspaceStore,
	workspaceEventLogs store.WorkspaceEventLogStore,
	producer queue.Producer,
	redisClient *redis.Client,
	redisGroup string,
) WorkspaceSetupService {
	return &workspaceSetupService{
		workspaces:         workspaces,
		workspaceEventLogs: workspaceEventLogs,
		producer:           producer,
		redis:              redisClient,
		redisGroup:         redisGroup,
	}
}

func (s *workspaceSetupService) Enqueue(ctx context.Context, workspaceID int64) (*WorkspaceSetupResult, error) {
	if workspaceID == 0 {
		return nil, fmt.Errorf("workspace id is required")
	}
	if s.redis == nil {
		return nil, fmt.Errorf("redis client not configured")
	}
	if s.redisGroup == "" {
		return nil, fmt.Errorf("redis group not configured")
	}

	workspace, err := s.workspaces.GetByID(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("fetching workspace: %w", err)
	}

	stream := queue.WorkspaceStreamName(workspace.OrganizationID, workspace.ID)
	if err := s.ensureConsumerGroup(ctx, stream); err != nil {
		return nil, fmt.Errorf("ensuring consumer group: %w", err)
	}

	runID := id.New()
	log := &model.WorkspaceEventLog{
		ID:             runID,
		WorkspaceID:    workspace.ID,
		OrganizationID: workspace.OrganizationID,
		EventType:      string(model.WorkspaceEventTypeSetup),
		Status:         string(model.WorkspaceEventStatusQueued),
	}

	if _, err := s.workspaceEventLogs.Create(ctx, log); err != nil {
		return nil, fmt.Errorf("creating workspace setup log: %w", err)
	}

	if err := s.producer.Enqueue(ctx, queue.Task{
		TaskType:        queue.TaskTypeWorkspaceSetup,
		WorkspaceID:     &workspace.ID,
		OrganizationID:  &workspace.OrganizationID,
		RunID:           &runID,
		Attempt:         1,
	}); err != nil {
		errMsg := err.Error()
		log.Status = string(model.WorkspaceEventStatusFailed)
		log.Error = &errMsg
		finishedAt := time.Now().UTC()
		_, _ = s.workspaceEventLogs.Update(ctx, log, nil, &finishedAt)
		return nil, fmt.Errorf("enqueueing workspace setup: %w", err)
	}

	return &WorkspaceSetupResult{RunID: runID}, nil
}

func (s *workspaceSetupService) ensureConsumerGroup(ctx context.Context, stream string) error {
	if stream == "" {
		return fmt.Errorf("stream is empty")
	}
	err := s.redis.XGroupCreateMkStream(ctx, stream, s.redisGroup, "0").Err()
	if err == nil {
		return nil
	}
	if errors.Is(err, redis.Nil) {
		return nil
	}
	if strings.Contains(err.Error(), "BUSYGROUP") {
		return nil
	}
	return err
}
