package service

import (
	"basegraph.app/relay/core/config"
	"basegraph.app/relay/internal/gap"
	"basegraph.app/relay/internal/llm"
	"basegraph.app/relay/internal/queue"
	"basegraph.app/relay/internal/service/integration"
	"basegraph.app/relay/internal/spec"
	"basegraph.app/relay/internal/store"
)

type Services struct {
	stores       *store.Stores
	txRunner     TxRunner
	workOSCfg    config.WorkOSConfig
	dashboardURL string
	webhookCfg   config.EventWebhookConfig
	eventIngest  EventIngestService
	llmClient    llm.Client
	gapDetector  gap.Detector
	specGen      spec.Generator
}

// Services is a factory that creates all service instances with their dependencies.
// It follows the factory pattern to centralize service construction and dependency injection.
//
// Production code uses this factory to get service instances:
//
//	services := service.NewServices(stores, txRunner, ...)
//	userService := services.Users()
//
// Tests use individual constructors (e.g., NewUserService) to inject mocks directly.
func NewServices(stores *store.Stores, txRunner TxRunner, workOSCfg config.WorkOSConfig, dashboardURL string, webhookCfg config.EventWebhookConfig, eventProducer queue.Producer, llmClient llm.Client) *Services {
	var gapDetector gap.Detector
	var specGen spec.Generator

	if llmClient != nil {
		gapDetector = gap.New(llmClient)
		specGen = spec.New(llmClient)
	}

	return &Services{
		stores:       stores,
		txRunner:     txRunner,
		workOSCfg:    workOSCfg,
		dashboardURL: dashboardURL,
		webhookCfg:   webhookCfg,
		eventIngest:  NewEventIngestService(stores, txRunner, eventProducer),
		llmClient:    llmClient,
		gapDetector:  gapDetector,
		specGen:      specGen,
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

func (s *Services) IntegrationCredentials() store.IntegrationCredentialStore {
	return s.stores.IntegrationCredentials()
}

func (s *Services) WebhookBaseURL() string {
	return s.webhookCfg.BaseURL
}

func (s *Services) Events() EventIngestService {
	return s.eventIngest
}

func (s *Services) GapDetector() gap.Detector {
	return s.gapDetector
}

func (s *Services) SpecGenerator() spec.Generator {
	return s.specGen
}
