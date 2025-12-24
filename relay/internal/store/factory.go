package store

import (
	"basegraph.app/relay/core/db/sqlc"
)

type Stores struct {
	queries *sqlc.Queries
}

func NewStores(queries *sqlc.Queries) *Stores {
	return &Stores{queries: queries}
}

func (s *Stores) Users() UserStore {
	return newUserStore(s.queries)
}

func (s *Stores) Organizations() OrganizationStore {
	return newOrganizationStore(s.queries)
}

func (s *Stores) Workspaces() WorkspaceStore {
	return newWorkspaceStore(s.queries)
}

func (s *Stores) Integrations() IntegrationStore {
	return newIntegrationStore(s.queries)
}

func (s *Stores) IntegrationCredentials() IntegrationCredentialStore {
	return newIntegrationCredentialStore(s.queries)
}

func (s *Stores) IntegrationConfigs() IntegrationConfigStore {
	return newIntegrationConfigStore(s.queries)
}

func (s *Stores) Repos() RepoStore {
	return newRepoStore(s.queries)
}

func (s *Stores) Sessions() SessionStore {
	return newSessionStore(s.queries)
}

func (s *Stores) Issues() IssueStore {
	return newIssueStore(s.queries)
}

func (s *Stores) EventLogs() EventLogStore {
	return newEventLogStore(s.queries)
}

func (s *Stores) Learnings() LearningStore {
	return newLearningStore(s.queries)
}

func (s *Stores) LLMEvals() LLMEvalStore {
	return newLLMEvalStore(s.queries)
}
