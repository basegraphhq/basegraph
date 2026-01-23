package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"basegraph.co/relay/internal/model"
	"basegraph.co/relay/internal/store"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type gitLabRepoProvider struct {
	client       *gitlab.Client
	integration  *model.Integration
	configStore  store.IntegrationConfigStore
}

func newGitLabRepoProvider(integration *model.Integration, credential *model.IntegrationCredential, configs store.IntegrationConfigStore) (*gitLabRepoProvider, error) {
	instanceURL := ""
	if integration.ProviderBaseURL != nil {
		instanceURL = *integration.ProviderBaseURL
	}
	baseURL := strings.TrimSuffix(instanceURL, "/") + "/api/v4"
	client, err := gitlab.NewClient(
		credential.AccessToken,
		gitlab.WithBaseURL(baseURL),
	)
	if err != nil {
		return nil, fmt.Errorf("creating gitlab client: %w", err)
	}

	return &gitLabRepoProvider{
		client:      client,
		integration: integration,
		configStore: configs,
	}, nil
}

func (p *gitLabRepoProvider) EnsureDeployKey(ctx context.Context, repo model.Repository, key DeployKeySpec) (int64, error) {
	projectID, err := strconv.ParseInt(repo.ExternalRepoID, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid project id: %w", err)
	}

	repoKeyCfg, err := p.loadRepoKeyConfig(ctx, repo.ExternalRepoID)
	if err != nil {
		return 0, err
	}
	if repoKeyCfg != nil && repoKeyCfg.PublicKey == key.PublicKey {
		return repoKeyCfg.DeployKeyID, nil
	}

	keys, _, err := p.client.DeployKeys.ListProjectDeployKeys(projectID, &gitlab.ListProjectDeployKeysOptions{}, gitlab.WithContext(ctx))
	if err != nil {
		return 0, fmt.Errorf("list deploy keys: %w", err)
	}

	for _, existing := range keys {
		if existing.Key == key.PublicKey || existing.Title == key.Title {
			if key.CanPush && !existing.CanPush {
				canPush := true
				_, _, err := p.client.DeployKeys.UpdateDeployKey(projectID, existing.ID, &gitlab.UpdateDeployKeyOptions{CanPush: &canPush}, gitlab.WithContext(ctx))
				if err != nil {
					return 0, fmt.Errorf("update deploy key: %w", err)
				}
			}
			cfg := buildDeployKeyConfig(existing.ID, key)
			if err := p.saveRepoKeyConfig(ctx, repo.ExternalRepoID, cfg); err != nil {
				return 0, err
			}
			if err := p.saveWorkspaceKeyConfig(ctx, cfg); err != nil {
				return 0, err
			}
			return existing.ID, nil
		}
	}

	workspaceCfg, err := p.loadWorkspaceKeyConfig(ctx)
	if err != nil {
		return 0, err
	}

	if workspaceCfg != nil {
		_, _, enableErr := p.client.DeployKeys.EnableDeployKey(projectID, workspaceCfg.DeployKeyID, gitlab.WithContext(ctx))
		if enableErr == nil {
			if key.CanPush {
				canPush := true
				_, _, err := p.client.DeployKeys.UpdateDeployKey(projectID, workspaceCfg.DeployKeyID, &gitlab.UpdateDeployKeyOptions{CanPush: &canPush}, gitlab.WithContext(ctx))
				if err != nil {
					return 0, fmt.Errorf("update deploy key: %w", err)
				}
			}
			cfg := buildDeployKeyConfig(workspaceCfg.DeployKeyID, key)
			if err := p.saveRepoKeyConfig(ctx, repo.ExternalRepoID, cfg); err != nil {
				return 0, err
			}
			return workspaceCfg.DeployKeyID, nil
		}
	}

	canPush := key.CanPush
	keyValue := key.PublicKey
	title := key.Title
	deployKey, _, err := p.client.DeployKeys.AddDeployKey(projectID, &gitlab.AddDeployKeyOptions{
		Key:     &keyValue,
		Title:   &title,
		CanPush: &canPush,
	}, gitlab.WithContext(ctx))
	if err != nil {
		return 0, fmt.Errorf("add deploy key: %w", err)
	}
	if deployKey == nil {
		return 0, fmt.Errorf("deploy key not returned")
	}

	cfg := buildDeployKeyConfig(deployKey.ID, key)
	if err := p.saveRepoKeyConfig(ctx, repo.ExternalRepoID, cfg); err != nil {
		return 0, err
	}
	if err := p.saveWorkspaceKeyConfig(ctx, cfg); err != nil {
		return 0, err
	}

	return deployKey.ID, nil
}

func (p *gitLabRepoProvider) CloneURL(repo model.Repository) (string, error) {
	host, err := p.resolveHost(repo)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("git@%s:%s.git", host, repo.Slug), nil
}

func (p *gitLabRepoProvider) resolveHost(repo model.Repository) (string, error) {
	if p.integration.ProviderBaseURL != nil && *p.integration.ProviderBaseURL != "" {
		return hostFromURL(*p.integration.ProviderBaseURL)
	}
	return hostFromURL(repo.URL)
}

func (p *gitLabRepoProvider) loadRepoKeyConfig(ctx context.Context, externalID string) (*deployKeyConfig, error) {
	key := fmt.Sprintf("deploy_key_repo:%s", externalID)
	cfg, err := p.configStore.GetByIntegrationAndKey(ctx, p.integration.ID, key)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	var value deployKeyConfig
	if err := json.Unmarshal(cfg.Value, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

func (p *gitLabRepoProvider) loadWorkspaceKeyConfig(ctx context.Context) (*deployKeyConfig, error) {
	cfg, err := p.configStore.GetByIntegrationAndKey(ctx, p.integration.ID, deployKeyWorkspaceConfigKey)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	var value deployKeyConfig
	if err := json.Unmarshal(cfg.Value, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

func (p *gitLabRepoProvider) saveRepoKeyConfig(ctx context.Context, externalID string, value deployKeyConfig) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("deploy_key_repo:%s", externalID)
	return saveIntegrationConfig(ctx, p.configStore, p.integration.ID, key, deployKeyConfigType, payload)
}

func (p *gitLabRepoProvider) saveWorkspaceKeyConfig(ctx context.Context, value deployKeyConfig) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return saveIntegrationConfig(ctx, p.configStore, p.integration.ID, deployKeyWorkspaceConfigKey, deployKeyWorkspaceType, payload)
}

func hostFromURL(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("empty url")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		parsed, err = url.Parse("https://" + raw)
		if err != nil {
			return "", fmt.Errorf("parse url: %w", err)
		}
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("missing host")
	}
	return parsed.Host, nil
}
