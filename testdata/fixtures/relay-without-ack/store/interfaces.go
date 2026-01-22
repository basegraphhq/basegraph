package store

import (
	"context"
	"errors"
	"time"

	"basegraph.co/relay/internal/model"
)

var ErrNotFound = errors.New("not found")

type UserStore interface {
	GetByID(ctx context.Context, id int64) (*model.User, error)
	GetByEmail(ctx context.Context, email string) (*model.User, error)
	GetByWorkOSID(ctx context.Context, workosID string) (*model.User, error)
	Upsert(ctx context.Context, user *model.User) error
	UpsertByWorkOSID(ctx context.Context, user *model.User) error
	Create(ctx context.Context, user *model.User) error
	Update(ctx context.Context, user *model.User) error
	Delete(ctx context.Context, id int64) error
}

type OrganizationStore interface {
	GetByID(ctx context.Context, id int64) (*model.Organization, error)
	GetBySlug(ctx context.Context, slug string) (*model.Organization, error)
	Create(ctx context.Context, org *model.Organization) error
	Update(ctx context.Context, org *model.Organization) error
	Delete(ctx context.Context, id int64) error // soft delete
	ListByAdminUser(ctx context.Context, userID int64) ([]model.Organization, error)
}

type WorkspaceStore interface {
	GetByID(ctx context.Context, id int64) (*model.Workspace, error)
	GetByOrgAndSlug(ctx context.Context, orgID int64, slug string) (*model.Workspace, error)
	Create(ctx context.Context, ws *model.Workspace) error
	Update(ctx context.Context, ws *model.Workspace) error
	Delete(ctx context.Context, id int64) error // soft delete
	ListByOrganization(ctx context.Context, orgID int64) ([]model.Workspace, error)
	ListByUser(ctx context.Context, userID int64) ([]model.Workspace, error)
}

type IntegrationStore interface {
	GetByID(ctx context.Context, id int64) (*model.Integration, error)
	GetByWorkspaceAndProvider(ctx context.Context, workspaceID int64, provider model.Provider) (*model.Integration, error)
	Create(ctx context.Context, integration *model.Integration) error
	Update(ctx context.Context, integration *model.Integration) error
	SetEnabled(ctx context.Context, id int64, enabled bool) error
	Delete(ctx context.Context, id int64) error
	ListByWorkspace(ctx context.Context, workspaceID int64) ([]model.Integration, error)
	ListByOrganization(ctx context.Context, orgID int64) ([]model.Integration, error)
	ListByCapability(ctx context.Context, workspaceID int64, capability model.Capability) ([]model.Integration, error)
}

type IntegrationCredentialStore interface {
	GetByID(ctx context.Context, id int64) (*model.IntegrationCredential, error)
	GetPrimaryByIntegration(ctx context.Context, integrationID int64) (*model.IntegrationCredential, error)
	GetByIntegrationAndUser(ctx context.Context, integrationID int64, userID int64) (*model.IntegrationCredential, error)
	Create(ctx context.Context, cred *model.IntegrationCredential) error
	UpdateTokens(ctx context.Context, id int64, accessToken string, refreshToken *string, expiresAt *time.Time) error
	SetAsPrimary(ctx context.Context, integrationID int64, credentialID int64) error
	Revoke(ctx context.Context, id int64) error
	RevokeAllByIntegration(ctx context.Context, integrationID int64) error
	Delete(ctx context.Context, id int64) error
	ListByIntegration(ctx context.Context, integrationID int64) ([]model.IntegrationCredential, error)
	ListActiveByIntegration(ctx context.Context, integrationID int64) ([]model.IntegrationCredential, error)
}

type IntegrationConfigStore interface {
	GetByID(ctx context.Context, id int64) (*model.IntegrationConfig, error)
	GetByIntegrationAndKey(ctx context.Context, integrationID int64, key string) (*model.IntegrationConfig, error)
	ListByIntegration(ctx context.Context, integrationID int64) ([]model.IntegrationConfig, error)
	ListByIntegrationAndType(ctx context.Context, integrationID int64, configType string) ([]model.IntegrationConfig, error)
	Create(ctx context.Context, config *model.IntegrationConfig) error
	Update(ctx context.Context, config *model.IntegrationConfig) error
	Upsert(ctx context.Context, config *model.IntegrationConfig) error
	Delete(ctx context.Context, id int64) error
	DeleteByIntegration(ctx context.Context, integrationID int64) error
}

type RepoStore interface {
	GetByID(ctx context.Context, id int64) (*model.Repository, error)
	GetByExternalID(ctx context.Context, integrationID int64, externalRepoID string) (*model.Repository, error)
	Create(ctx context.Context, repo *model.Repository) error
	Update(ctx context.Context, repo *model.Repository) error
	Delete(ctx context.Context, id int64) error
	DeleteByIntegration(ctx context.Context, integrationID int64) error
	ListByWorkspace(ctx context.Context, workspaceID int64) ([]model.Repository, error)
	ListByIntegration(ctx context.Context, integrationID int64) ([]model.Repository, error)
}

type SessionStore interface {
	GetByID(ctx context.Context, id int64) (*model.Session, error)
	GetValid(ctx context.Context, id int64) (*model.Session, error) // checks expiry
	Create(ctx context.Context, session *model.Session) error
	Delete(ctx context.Context, id int64) error
	DeleteByUser(ctx context.Context, userID int64) error
	DeleteExpired(ctx context.Context) error
	ListByUser(ctx context.Context, userID int64) ([]model.Session, error)
}

type IssueStore interface {
	Upsert(ctx context.Context, issue *model.Issue) (*model.Issue, error)
	GetByID(ctx context.Context, id int64) (*model.Issue, error)
	GetByIntegrationAndExternalID(ctx context.Context, integrationID int64, externalIssueID string) (*model.Issue, error)

	// Issue-centric processing state transitions
	// QueueIfIdle queues an issue for processing, with automatic stuck issue recovery.
	// Returns (true, nil) if queued, (false, nil) if already being processed (within 15 min).
	// Also recovers stuck issues: if processing/queued for >15 min, resets and queues.
	QueueIfIdle(ctx context.Context, issueID int64) (queued bool, err error)
	// ClaimQueued atomically transitions an issue from 'queued' to 'processing'.
	// Returns (true, issue) if claimed, (false, nil) if already claimed by another worker.
	ClaimQueued(ctx context.Context, issueID int64) (claimed bool, issue *model.Issue, err error)
	// SetIdle transitions an issue from 'processing' to 'idle'.
	SetIdle(ctx context.Context, issueID int64) error
}

type EventLogStore interface {
	Create(ctx context.Context, log *model.EventLog) (*model.EventLog, error)
	CreateOrGet(ctx context.Context, log *model.EventLog) (*model.EventLog, bool, error)
	GetByID(ctx context.Context, id int64) (*model.EventLog, error)
	ListUnprocessed(ctx context.Context, limit int32) ([]model.EventLog, error)
	MarkProcessed(ctx context.Context, id int64) error
	MarkFailed(ctx context.Context, id int64, errMsg string) error

	// Issue-centric batch operations
	// ListUnprocessedByIssue returns all unprocessed events for an issue, ordered by created_at.
	ListUnprocessedByIssue(ctx context.Context, issueID int64) ([]model.EventLog, error)
	// MarkBatchProcessed marks multiple event logs as processed atomically.
	MarkBatchProcessed(ctx context.Context, ids []int64) error
}

type LearningStore interface {
	GetByID(ctx context.Context, id int64) (*model.Learning, error)
	GetByWorkspaceAndType(ctx context.Context, workspaceID int64, learningType string) (*model.Learning, error)
	Create(ctx context.Context, learning *model.Learning) error
	Update(ctx context.Context, learning *model.Learning) error
	Delete(ctx context.Context, id int64) error
	ListByWorkspace(ctx context.Context, workspaceID int64) ([]model.Learning, error)
	ListByWorkspaceAndType(ctx context.Context, workspaceID int64, learningType string) ([]model.Learning, error)
}

type LLMEvalStore interface {
	Create(ctx context.Context, eval *model.LLMEval) (*model.LLMEval, error)
	GetByID(ctx context.Context, id int64) (*model.LLMEval, error)
	ListByIssue(ctx context.Context, issueID int64) ([]model.LLMEval, error)
	ListByStage(ctx context.Context, stage string, limit int32) ([]model.LLMEval, error)
	ListUnrated(ctx context.Context, stage string, limit int32) ([]model.LLMEval, error)
	Rate(ctx context.Context, id int64, rating int, notes string, ratedByUserID int64) error
	SetExpected(ctx context.Context, id int64, expectedJSON []byte, evalScore float64) error
	GetStats(ctx context.Context, stage string, since time.Time) (*model.LLMEvalStats, error)
}

type GapStore interface {
	Create(ctx context.Context, gap model.Gap) (model.Gap, error)
	GetByID(ctx context.Context, id int64) (model.Gap, error)
	ListByIssue(ctx context.Context, issueID int64) ([]model.Gap, error)
	ListOpenByIssue(ctx context.Context, issueID int64) ([]model.Gap, error)
	Resolve(ctx context.Context, id int64) (model.Gap, error)
	Skip(ctx context.Context, id int64) (model.Gap, error)
	SetLearning(ctx context.Context, id int64, learningID int64) (model.Gap, error)
	CountOpenBlocking(ctx context.Context, issueID int64) (int64, error)
}
