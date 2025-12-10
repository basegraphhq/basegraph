package service

import (
	"basegraph.app/relay/core/config"
	"basegraph.app/relay/internal/service/integration"
	"basegraph.app/relay/internal/store"
)

type Services struct {
	stores       *store.Stores
	txRunner     TxRunner
	workOSCfg    config.WorkOSConfig
	dashboardURL string
}

func NewServices(stores *store.Stores, txRunner TxRunner, workOSCfg config.WorkOSConfig, dashboardURL string) *Services {
	return &Services{
		stores:       stores,
		txRunner:     txRunner,
		workOSCfg:    workOSCfg,
		dashboardURL: dashboardURL,
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
		s.workOSCfg,
		s.dashboardURL,
	)
}

func (s *Services) GitLab() integration.GitLabService {
	return integration.NewGitLabService()
}
