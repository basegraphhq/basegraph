package service

import (
	"context"
	"fmt"

	"basegraph.app/relay/common"
	"basegraph.app/relay/common/id"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/store"
)

type OrganizationService interface {
	Create(ctx context.Context, name string, slug *string, adminUserID int64) (*model.Organization, error)
}

type organizationService struct {
	orgStore store.OrganizationStore
}

func NewOrganizationService(orgStore store.OrganizationStore) OrganizationService {
	return &organizationService{orgStore: orgStore}
}

func (s *organizationService) Create(ctx context.Context, name string, slug *string, adminUserID int64) (*model.Organization, error) {
	finalSlug, err := s.ensureSlug(ctx, name, slug)
	if err != nil {
		return nil, err
	}

	org := &model.Organization{
		ID:          id.New(),
		AdminUserID: adminUserID,
		Name:        name,
		Slug:        finalSlug,
	}

	if err := s.orgStore.Create(ctx, org); err != nil {
		return nil, fmt.Errorf("creating organization: %w", err)
	}

	return org, nil
}

func (s *organizationService) ensureSlug(ctx context.Context, name string, slug *string) (string, error) {
	input := name
	if slug != nil && *slug != "" {
		input = *slug
	}

	base, err := common.Slugify(input, "org")
	if err != nil {
		return "", fmt.Errorf("generating slug: %w", err)
	}

	// Fast path
	if _, err := s.orgStore.GetBySlug(ctx, base); err != nil {
		if err == store.ErrNotFound {
			return base, nil
		}
		return "", fmt.Errorf("checking slug availability: %w", err)
	}

	// Add numeric suffix until available
	for i := 1; i <= 20; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		_, err := s.orgStore.GetBySlug(ctx, candidate)
		if err == store.ErrNotFound {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("checking slug availability: %w", err)
		}
	}

	return "", fmt.Errorf("unable to find available slug for %q", base)
}
