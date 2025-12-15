package service

import (
	"context"
	"errors"
	"fmt"

	"basegraph.app/api-server/common"
	"basegraph.app/api-server/common/id"
	"basegraph.app/api-server/internal/model"
	"basegraph.app/api-server/internal/store"
)

type OrganizationService interface {
	Create(ctx context.Context, name string, slug *string, adminUserID int64) (*model.Organization, error)
}

type organizationService struct {
	tx TxRunner
}

func NewOrganizationService(tx TxRunner) OrganizationService {
	return &organizationService{
		tx: tx,
	}
}

func (s *organizationService) Create(ctx context.Context, name string, slug *string, adminUserID int64) (*model.Organization, error) {
	var createdOrg *model.Organization

	err := s.tx.WithTx(ctx, func(stores StoreProvider) error {
		orgStore := stores.Organizations()
		workspaceStore := stores.Workspaces()

		finalSlug, err := s.ensureOrgSlug(ctx, orgStore, name, slug)
		if err != nil {
			return err
		}

		org := &model.Organization{
			ID:          id.New(),
			AdminUserID: adminUserID,
			Name:        name,
			Slug:        finalSlug,
		}

		if err := orgStore.Create(ctx, org); err != nil {
			return fmt.Errorf("creating organization: %w", err)
		}

		if err := s.createDefaultWorkspace(ctx, workspaceStore, org, adminUserID); err != nil {
			return fmt.Errorf("creating default workspace: %w", err)
		}

		createdOrg = org
		return nil
	})
	if err != nil {
		return nil, err
	}

	return createdOrg, nil
}

func (s *organizationService) ensureOrgSlug(ctx context.Context, orgStore store.OrganizationStore, name string, slug *string) (string, error) {
	input := name
	if slug != nil && *slug != "" {
		input = *slug
	}

	base, err := common.Slugify(input, "org")
	if err != nil {
		return "", fmt.Errorf("generating slug: %w", err)
	}

	// Fast path
	if _, err := orgStore.GetBySlug(ctx, base); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return base, nil
		}
		return "", fmt.Errorf("checking slug availability: %w", err)
	}

	// Add numeric suffix until available
	for i := 1; i <= 20; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		_, err := orgStore.GetBySlug(ctx, candidate)
		if errors.Is(err, store.ErrNotFound) {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("checking slug availability: %w", err)
		}
	}

	return "", fmt.Errorf("unable to find available slug for %q", base)
}

func (s *organizationService) createDefaultWorkspace(ctx context.Context, workspaceStore store.WorkspaceStore, org *model.Organization, adminUserID int64) error {
	wsSlug, err := s.ensureWorkspaceSlug(ctx, workspaceStore, org.ID, org.Name)
	if err != nil {
		return err
	}

	ws := &model.Workspace{
		ID:             id.New(),
		AdminUserID:    adminUserID,
		OrganizationID: org.ID,
		UserID:         adminUserID,
		Name:           fmt.Sprintf("%s workspace", org.Name),
		Slug:           wsSlug,
	}

	if err := workspaceStore.Create(ctx, ws); err != nil {
		return fmt.Errorf("creating workspace: %w", err)
	}

	return nil
}

func (s *organizationService) ensureWorkspaceSlug(ctx context.Context, workspaceStore store.WorkspaceStore, orgID int64, orgName string) (string, error) {
	base, err := common.Slugify(orgName, "workspace")
	if err != nil {
		return "", fmt.Errorf("generating workspace slug: %w", err)
	}

	if _, err := workspaceStore.GetByOrgAndSlug(ctx, orgID, base); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return base, nil
		}
		return "", fmt.Errorf("checking workspace slug availability: %w", err)
	}

	for i := 1; i <= 20; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		_, err := workspaceStore.GetByOrgAndSlug(ctx, orgID, candidate)
		if errors.Is(err, store.ErrNotFound) {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("checking workspace slug availability: %w", err)
		}
	}

	return "", fmt.Errorf("unable to find available workspace slug for %q", base)
}
