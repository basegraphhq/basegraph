package service

import (
	"context"
	"fmt"

	"basegraph.app/relay/core/config"
	"basegraph.app/relay/internal/service/integration"
	"basegraph.app/relay/internal/store"
	"basegraph.app/relay/internal/queue"
)

type Services struct {
	stores       *store.Stores
	txRunner     TxRunner
	workOSCfg    config.WorkOSConfig
	dashboardURL string
	webhookCfg   config.EventWebhookConfig
	eventIngest  EventIngestService
}

func NewServices(stores *store.Stores, txRunner TxRunner, workOSCfg config.WorkOSConfig, dashboardURL string, webhookCfg config.EventWebhookConfig, eventProducer queue.Producer) *Services {
	return &Services{
		stores:       stores,
		txRunner:     txRunner,
		workOSCfg:    workOSCfg,
		dashboardURL: dashboardURL,
		webhookCfg:   webhookCfg,
		eventIngest:  NewEventIngestService(stores, txRunner, eventProducer, nil),
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

type gitLabTxRunnerAdapter struct {
	tx TxRunner
}

func (a *gitLabTxRunnerAdapter) WithTx(ctx context.Context, fn func(stores integration.StoreProvider) error) error {
	return a.tx.WithTx(ctx, func(sp StoreProvider) error {
		stores, ok := sp.(*store.Stores)
		if !ok {
			return fmt.Errorf("unexpected store provider type %T", sp)
		}
		return fn(stores)
	})
}
