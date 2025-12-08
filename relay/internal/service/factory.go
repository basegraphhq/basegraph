package service

import (
	"basegraph.app/relay/internal/store"
)

type Services struct {
	stores *store.Stores
}

func NewServices(stores *store.Stores) *Services {
	return &Services{stores: stores}
}

func (s *Services) Users() UserService {
	return NewUserService(s.stores.Users())
}
