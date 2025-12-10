package service

import (
	"basegraph.app/relay/internal/service/integration"
	"basegraph.app/relay/internal/store"
)

type Services struct {
	stores   *store.Stores
	txRunner TxRunner
}

func NewServices(stores *store.Stores, txRunner TxRunner) *Services {
	return &Services{
		stores:   stores,
		txRunner: txRunner,
	}
}

func (s *Services) Users() UserService {
	return NewUserService(s.stores.Users(), s.stores.Organizations())
}

func (s *Services) Organizations() OrganizationService {
	return NewOrganizationService(s.txRunner)
}

func (s *Services) GitLab() integration.GitLabService {
	return integration.NewGitLabService()
}
