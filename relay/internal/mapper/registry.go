package mapper

import (
	"fmt"
	"sync"
)

type MapperRegistry struct {
	mu      sync.RWMutex
	mappers map[string]EventMapper
}

func NewMapperRegistry() *MapperRegistry {
	registry := &MapperRegistry{
		mappers: make(map[string]EventMapper),
	}

	registry.Register("gitlab", NewGitLabEventMapper())

	return registry
}

func (r *MapperRegistry) Register(provider string, mapper EventMapper) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mappers[provider] = mapper
}

func (r *MapperRegistry) Get(provider string) (EventMapper, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mapper, exists := r.mappers[provider]
	if !exists {
		return nil, fmt.Errorf("unsupported provider: %s (supported: gitlab, github, linear)", provider)
	}

	return mapper, nil
}

func (r *MapperRegistry) MustGet(provider string) EventMapper {
	mapper, err := r.Get(provider)
	if err != nil {
		panic(err)
	}
	return mapper
}
