package service

import (
	"basegraph.co/relay/core/config"
	"basegraph.co/relay/internal/model"
	"basegraph.co/relay/internal/queue"
	"basegraph.co/relay/internal/service/integration"
	tracker "basegraph.co/relay/internal/service/issue_tracker"
	"basegraph.co/relay/internal/store"
	"github.com/redis/go-redis/v9"
)

type ServicesConfig struct {
	Stores        *store.Stores
	TxRunner      TxRunner
	WorkOS        config.WorkOSConfig
	DashboardURL  string
	WebhookCfg    config.EventWebhookConfig
	EventProducer queue.Producer
	RedisClient   *redis.Client
	RedisGroup    string
}

type Services struct {
	stores       *store.Stores
	txRunner     TxRunner
	workOSCfg    config.WorkOSConfig
	dashboardURL string
	webhookCfg   config.EventWebhookConfig
	producer     queue.Producer
	redis        *redis.Client
	redisGroup   string
}

// Services is a factory that initializes all services with their dependencies.
// Usage:
//
//	services := service.NewServices(service.ServicesConfig{...})
//	userService := services.Users()
//
// Tests use individual constructors (e.g., NewUserService) to inject mocks directly.
func NewServices(cfg ServicesConfig) *Services {
	return &Services{
		stores:       cfg.Stores,
		txRunner:     cfg.TxRunner,
		producer:     cfg.EventProducer,
		workOSCfg:    cfg.WorkOS,
		dashboardURL: cfg.DashboardURL,
		webhookCfg:   cfg.WebhookCfg,
		redis:        cfg.RedisClient,
		redisGroup:   cfg.RedisGroup,
	}
}

func (s *Services) Users() UserService {
	return NewUserService(s.stores.Users(), s.stores.Organizations())
}

func (s *Services) Organizations() OrganizationService {
	return NewOrganizationService(s.txRunner)
}

func (s *Services) Auth() AuthService {
	return NewAuthService(
		s.stores.Users(),
		s.stores.Sessions(),
		s.stores.Organizations(),
		s.stores.Workspaces(),
		s.workOSCfg,
		s.dashboardURL,
	)
}

func (s *Services) GitLab() integration.GitLabService {
	return integration.NewGitLabService(
		s.stores,
		&gitLabTxRunnerAdapter{tx: s.txRunner},
	)
}

func (s *Services) IntegrationCredentials() IntegrationCredentialService {
	return NewIntegrationCredentialService(s.stores.IntegrationCredentials())
}

func (s *Services) WebhookBaseURL() string {
	return s.webhookCfg.BaseURL
}

func (s *Services) GitlabIssueTracker() tracker.IssueTrackerService {
	return tracker.NewGitLabIssueTrackerService(s.stores.Integrations(), s.stores.IntegrationCredentials())
}

func (s *Services) EngagementDetector() EngagementDetector {
	return NewEngagementDetector(
		s.stores.IntegrationConfigs(),
		s.IssueTrackers(),
	)
}

func (s *Services) IssueTrackers() map[model.Provider]tracker.IssueTrackerService {
	return map[model.Provider]tracker.IssueTrackerService{
		model.ProviderGitLab: s.GitlabIssueTracker(),
		// model.ProviderGitHub: s.GithubIssueTracker(),
		// model.ProviderLinear: s.LinearIssueTracker(),
		// model.ProviderJira:   s.JiraIssueTracker(),
	}
}

func (s *Services) EventIngest() EventIngestService {
	return NewEventIngestService(
		s.stores.Integrations(),
		s.stores.Issues(),
		s.txRunner,
		s.producer,
		s.IssueTrackers(),
		s.EngagementDetector(),
	)
}

func (s *Services) RepoSync() RepoSyncService {
	return NewRepoSyncService(
		s.stores.Integrations(),
		s.stores.Repos(),
		s.stores.WorkspaceEventLogs(),
		s.producer,
	)
}

func (s *Services) WorkspaceSetup() WorkspaceSetupService {
	return NewWorkspaceSetupService(
		s.stores.Workspaces(),
		s.stores.WorkspaceEventLogs(),
		s.producer,
		s.redis,
		s.redisGroup,
	)
}


func (s *Services) Invitations() InvitationService {
	return NewInvitationService(s.stores.Invitations(), s.dashboardURL)
}
