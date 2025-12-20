package service

import (
	"basegraph.app/relay/core/config"
	"basegraph.app/relay/internal/queue"
	"basegraph.app/relay/internal/service/integration"
	"basegraph.app/relay/internal/store"
)

type ServicesConfig struct {
	Stores        *store.Stores
	TxRunner      TxRunner
	WorkOS        config.WorkOSConfig
	DashboardURL  string
	WebhookCfg    config.EventWebhookConfig
	EventProducer queue.Producer
}

type Services struct {
	stores                *store.Stores
	txRunner              TxRunner
	workOSCfg             config.WorkOSConfig
	dashboardURL          string
	webhookCfg            config.EventWebhookConfig
	eventIngest           EventIngestService
	integrationCredential IntegrationCredentialService
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
		stores:                cfg.Stores,
		txRunner:              cfg.TxRunner,
		workOSCfg:             cfg.WorkOS,
		dashboardURL:          cfg.DashboardURL,
		webhookCfg:            cfg.WebhookCfg,
		eventIngest:           NewEventIngestService(cfg.Stores.Integrations(), cfg.TxRunner, cfg.EventProducer),
		integrationCredential: NewIntegrationCredentialService(cfg.Stores.IntegrationCredentials()),
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
	return s.integrationCredential
}

func (s *Services) WebhookBaseURL() string {
	return s.webhookCfg.BaseURL
}

func (s *Services) Events() EventIngestService {
	return s.eventIngest
}
