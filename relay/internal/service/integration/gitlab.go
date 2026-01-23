package integration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"basegraph.co/relay/common/id"
	"basegraph.co/relay/internal/model"
	"basegraph.co/relay/internal/store"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type GitLabService interface {
	ListProjects(ctx context.Context, instanceURL, token string) ([]GitLabProject, error)
	SetupIntegration(ctx context.Context, params SetupIntegrationParams) (*SetupResult, error)
	EnableRepositories(ctx context.Context, params EnableRepositoriesParams) (*EnableRepositoriesResult, error)
	ListEnabledProjectIDs(ctx context.Context, workspaceID int64) ([]int64, error)
	Status(ctx context.Context, workspaceID int64) (*StatusResult, error)
	RefreshIntegration(ctx context.Context, workspaceID int64, webhookBaseURL string) (*SetupResult, error)
}

type GitLabProject struct {
	Name        string // e.g. "api-service"
	PathWithNS  string // e.g. "acme-corp/backend/api-service"
	WebURL      string // e.g. "https://gitlab.com/acme-corp/backend/api-service"
	Description string // e.g. "API service for the Acme Corp backend"
	DefaultBranch string
	ID          int64
}

type SetupIntegrationParams struct {
	InstanceURL    string
	Token          string
	WebhookBaseURL string
	WorkspaceID    int64
	OrganizationID int64
	SetupByUserID  int64
}

type SetupResult struct {
	Projects          []GitLabProject
	Errors            []string
	IntegrationID     int64
	RepositoriesAdded int
	WebhooksCreated   int
	IsNewIntegration  bool
}

type EnableRepositoriesParams struct {
	WorkspaceID    int64
	ProjectIDs     []int64
	WebhookBaseURL string
}

type EnableRepositoriesResult struct {
	Projects          []GitLabProject
	Errors            []string
	IntegrationID     int64
	RepositoriesAdded int
	WebhooksCreated   int
}

type StatusResult struct {
	IntegrationID     *int64
	UpdatedAt         *time.Time
	Errors            []string
	WebhooksCreated   int
	RepositoriesAdded int
	ReposCount        int
	Connected         bool
	Synced            bool
}

// StoreProvider is the minimal view of stores needed by GitLab when running
// inside a transaction. It is implemented by *store.Stores in production and
// by fakes in tests.
type StoreProvider interface {
	Integrations() store.IntegrationStore
	IntegrationCredentials() store.IntegrationCredentialStore
	IntegrationConfigs() store.IntegrationConfigStore
	Repos() store.RepoStore
}

// TxRunner is a narrow transaction runner dependency for the GitLab service.
// It is intentionally defined here to avoid a dependency cycle back into the
// main service package while still allowing transactional operations.
type TxRunner interface {
	WithTx(ctx context.Context, fn func(stores StoreProvider) error) error
}

type gitLabService struct {
	txRunner  TxRunner
	stores    *store.Stores
	repoStore store.RepoStore
}

func NewGitLabService(stores *store.Stores, txRunner TxRunner) GitLabService {
	return &gitLabService{
		txRunner:  txRunner,
		stores:    stores,
		repoStore: stores.Repos(),
	}
}

func (s *gitLabService) ListProjects(ctx context.Context, instanceURL, token string) ([]GitLabProject, error) {
	client, err := s.newClient(instanceURL, token)
	if err != nil {
		return nil, err
	}

	opts := &gitlab.ListProjectsOptions{
		MinAccessLevel: gitlab.Ptr(gitlab.MaintainerPermissions),
		ListOptions: gitlab.ListOptions{
			Page:    1,
			PerPage: 100,
		},
	}

	var projects []GitLabProject

	for {
		pageProjects, resp, err := client.Projects.ListProjects(opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, err
		}

		for _, p := range pageProjects {
			projects = append(projects, GitLabProject{
				ID:          p.ID,
				Name:        p.Name,
				PathWithNS:  p.PathWithNamespace,
				WebURL:      p.WebURL,
				Description: p.Description,
				DefaultBranch: p.DefaultBranch,
			})
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return projects, nil
}

type webhookConfigValue struct {
	ProjectPath string   `json:"project_path"`
	Events      []string `json:"events"`
	WebhookID   int64    `json:"webhook_id"`
}

func (s *gitLabService) SetupIntegration(ctx context.Context, params SetupIntegrationParams) (*SetupResult, error) {
	if params.WebhookBaseURL == "" {
		return nil, fmt.Errorf("webhook base URL is required")
	}

	client, err := s.newClient(params.InstanceURL, params.Token)
	if err != nil {
		return nil, err
	}

	projects, err := s.ListProjects(ctx, params.InstanceURL, params.Token)
	if err != nil {
		return nil, fmt.Errorf("validating token: %w", err)
	}

	projectNames := make([]string, 0, len(projects))
	for _, p := range projects {
		projectNames = append(projectNames, p.PathWithNS)
	}
	slog.InfoContext(ctx, "gitlab projects fetched",
		"count", len(projects),
		"projects", projectNames,
		"instance_url", params.InstanceURL,
	)

	if len(projects) == 0 {
		return nil, fmt.Errorf("no projects found with maintainer access - ensure the token belongs to a user with Maintainer role on at least one project")
	}

	var (
		integration      *model.Integration
		webhookSecret    string
		isNewIntegration bool
	)

	err = s.txRunner.WithTx(ctx, func(stores StoreProvider) error {
		existing, err := stores.Integrations().GetByWorkspaceAndProvider(ctx, params.WorkspaceID, model.ProviderGitLab)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("fetching integration: %w", err)
		}

		if errors.Is(err, store.ErrNotFound) {
			isNewIntegration = true
			integration = &model.Integration{
				ID:             id.New(),
				WorkspaceID:    params.WorkspaceID,
				OrganizationID: params.OrganizationID,
				SetupByUserID:  params.SetupByUserID,
				Provider:       model.ProviderGitLab,
				Capabilities:   model.ProviderGitLab.DefaultCapabilities(),
				ProviderBaseURL: func(u string) *string {
					if u == "" {
						return nil
					}
					return &u
				}(params.InstanceURL),
				IsEnabled: true,
			}

			if err := stores.Integrations().Create(ctx, integration); err != nil {
				return fmt.Errorf("creating integration: %w", err)
			}

			// Store the GitLab Personal Access Token (PAT) as the primary integration credential.
			// This credential is required for authenticating API requests such as creating webhooks.
			pat := &model.IntegrationCredential{
				ID:             id.New(),
				IntegrationID:  integration.ID,
				UserID:         &params.SetupByUserID,
				CredentialType: model.CredentialTypeAPIKey,
				AccessToken:    params.Token,
				IsPrimary:      true,
			}

			if err := stores.IntegrationCredentials().Create(ctx, pat); err != nil {
				return fmt.Errorf("storing access token: %w", err)
			}
		} else {
			integration = existing

			primary, err := stores.IntegrationCredentials().GetPrimaryByIntegration(ctx, integration.ID)
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("fetching primary credential: %w", err)
			}

			if errors.Is(err, store.ErrNotFound) {
				pat := &model.IntegrationCredential{
					ID:             id.New(),
					IntegrationID:  integration.ID,
					UserID:         &params.SetupByUserID,
					CredentialType: model.CredentialTypeAPIKey,
					AccessToken:    params.Token,
					IsPrimary:      true,
				}

				if err := stores.IntegrationCredentials().Create(ctx, pat); err != nil {
					return fmt.Errorf("storing access token: %w", err)
				}
			} else if primary != nil && primary.AccessToken != params.Token {
				if err := stores.IntegrationCredentials().UpdateTokens(ctx, primary.ID, params.Token, primary.RefreshToken, primary.TokenExpiresAt); err != nil {
					return fmt.Errorf("updating access token: %w", err)
				}
			}
		}

		if webhookSecret == "" {
			activeCreds, err := stores.IntegrationCredentials().ListActiveByIntegration(ctx, integration.ID)
			if err != nil {
				return fmt.Errorf("listing credentials: %w", err)
			}

			for _, cred := range activeCreds {
				if cred.CredentialType == model.CredentialTypeWebhookSecret {
					webhookSecret = cred.AccessToken
					break
				}
			}
		}

		if webhookSecret == "" {
			webhookSecret, err = generateSecret()
			if err != nil {
				return fmt.Errorf("generating webhook secret: %w", err)
			}

			secretCred := &model.IntegrationCredential{
				ID:             id.New(),
				IntegrationID:  integration.ID,
				CredentialType: model.CredentialTypeWebhookSecret,
				AccessToken:    webhookSecret,
				IsPrimary:      false,
			}

			if err := stores.IntegrationCredentials().Create(ctx, secretCred); err != nil {
				return fmt.Errorf("storing webhook secret: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Fetch the service account (bot) identity for @mention detection
	user, _, err := client.Users.CurrentUser(gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("fetching service account user: %w", err)
	}

	serviceAccountValue, err := json.Marshal(struct {
		Username string `json:"username"`
		UserID   int64  `json:"user_id"`
	}{
		Username: user.Username,
		UserID:   int64(user.ID),
	})
	if err != nil {
		return nil, fmt.Errorf("serializing service account config: %w", err)
	}

	if err := s.txRunner.WithTx(ctx, func(stores StoreProvider) error {
		cfgStore := stores.IntegrationConfigs()
		existing, err := cfgStore.GetByIntegrationAndKey(ctx, integration.ID, "service_account")
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("fetching service account config: %w", err)
		}

		if err == nil {
			existing.Value = serviceAccountValue
			return cfgStore.Update(ctx, existing)
		}

		config := &model.IntegrationConfig{
			ID:            id.New(),
			IntegrationID: integration.ID,
			Key:           "service_account",
			Value:         serviceAccountValue,
			ConfigType:    "identity",
		}
		return cfgStore.Create(ctx, config)
	}); err != nil {
		return nil, fmt.Errorf("storing service account config: %w", err)
	}

	slog.InfoContext(ctx, "stored service account identity",
		"integration_id", integration.ID,
		"username", user.Username,
		"user_id", user.ID,
	)

	return &SetupResult{
		IntegrationID:     integration.ID,
		IsNewIntegration:  isNewIntegration,
		Projects:          projects,
		RepositoriesAdded: 0,
		WebhooksCreated:   0,
		Errors:            nil,
	}, nil
}

func (s *gitLabService) EnableRepositories(ctx context.Context, params EnableRepositoriesParams) (*EnableRepositoriesResult, error) {
	if params.WebhookBaseURL == "" {
		return nil, fmt.Errorf("webhook base URL is required")
	}

	var (
		integration      *model.Integration
		primaryCred      *model.IntegrationCredential
		webhookSecret    string
		webhookConfigs   map[string]webhookConfigValue
	)

	if err := s.txRunner.WithTx(ctx, func(stores StoreProvider) error {
		var err error
		integration, err = stores.Integrations().GetByWorkspaceAndProvider(ctx, params.WorkspaceID, model.ProviderGitLab)
		if err != nil {
			return fmt.Errorf("fetching integration: %w", err)
		}

		primaryCred, err = stores.IntegrationCredentials().GetPrimaryByIntegration(ctx, integration.ID)
		if err != nil {
			return fmt.Errorf("fetching primary credential: %w", err)
		}

		webhookSecret, err = ensureWebhookSecret(ctx, stores.IntegrationCredentials(), integration.ID)
		if err != nil {
			return fmt.Errorf("ensuring webhook secret: %w", err)
		}

		configs, err := stores.IntegrationConfigs().ListByIntegrationAndType(ctx, integration.ID, "webhook")
		if err != nil {
			return fmt.Errorf("listing webhook configs: %w", err)
		}

		webhookConfigs = make(map[string]webhookConfigValue, len(configs))
		for _, cfg := range configs {
			var val webhookConfigValue
			if err := json.Unmarshal(cfg.Value, &val); err != nil {
				continue
			}
			webhookConfigs[cfg.Key] = val
		}
		return nil
	}); err != nil {
		return nil, err
	}

	instanceURL := ""
	if integration.ProviderBaseURL != nil {
		instanceURL = *integration.ProviderBaseURL
	}

	projects, err := s.ListProjects(ctx, instanceURL, primaryCred.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}

	projectByID := make(map[int64]GitLabProject, len(projects))
	for _, project := range projects {
		projectByID[project.ID] = project
	}

	selectedProjects := make([]GitLabProject, 0, len(params.ProjectIDs))
	var errs []string
	for _, id := range params.ProjectIDs {
		project, ok := projectByID[id]
		if !ok {
			errs = append(errs, fmt.Sprintf("project %d: not found", id))
			continue
		}
		selectedProjects = append(selectedProjects, project)
	}

	webhookURL := strings.TrimSuffix(params.WebhookBaseURL, "/") + fmt.Sprintf("/webhooks/gitlab/%d", integration.ID)
	webhookName := "Relay"
	webhookDescription := "Relay outbound webhook"
	events := []string{"issues_events", "merge_requests_events", "note_events", "wiki_page_events", "push_events"}

	existingRepos, err := s.repoStore.ListByIntegration(ctx, integration.ID)
	if err != nil {
		return nil, fmt.Errorf("listing repositories: %w", err)
	}

	existingByExternal := make(map[string]model.Repository, len(existingRepos))
	for _, repo := range existingRepos {
		existingByExternal[repo.ExternalRepoID] = repo
	}

	selectedExternal := make(map[string]struct{}, len(selectedProjects))
	for _, project := range selectedProjects {
		externalID := strconv.FormatInt(project.ID, 10)
		selectedExternal[externalID] = struct{}{}
	}

	for _, repo := range existingRepos {
		if _, ok := selectedExternal[repo.ExternalRepoID]; ok {
			continue
		}
		if !repo.IsEnabled {
			continue
		}
		if _, err := s.repoStore.SetEnabled(ctx, repo.ID, false); err != nil {
			errs = append(errs, fmt.Sprintf("repo %s: disable failed: %v", repo.Slug, err))
		}
	}

	var webhooksCreated int
	var repositoriesAdded int

	client, err := s.newClient(instanceURL, primaryCred.AccessToken)
	if err != nil {
		return nil, err
	}

	for _, project := range selectedProjects {
		externalID := strconv.FormatInt(project.ID, 10)
		repo, exists := existingByExternal[externalID]

		repoDefaults := func() *string {
			if strings.TrimSpace(project.DefaultBranch) == "" {
				return nil
			}
			branch := project.DefaultBranch
			return &branch
		}()

		if exists {
			repo.Name = project.Name
			repo.Slug = project.PathWithNS
			repo.URL = project.WebURL
			repo.Description = nil
			if desc := strings.TrimSpace(project.Description); desc != "" {
				repo.Description = &desc
			}
			if err := s.repoStore.Update(ctx, &repo); err != nil {
				errs = append(errs, fmt.Sprintf("project %s: updating repository: %v", project.PathWithNS, err))
			} else if _, err := s.repoStore.SetEnabled(ctx, repo.ID, true); err != nil {
				errs = append(errs, fmt.Sprintf("project %s: enabling repository: %v", project.PathWithNS, err))
			}
			if repoDefaults != nil {
				if _, err := s.repoStore.UpdateDefaultBranch(ctx, repo.ID, repoDefaults); err != nil {
					errs = append(errs, fmt.Sprintf("project %s: updating default branch: %v", project.PathWithNS, err))
				}
			}
		} else {
			repo := &model.Repository{
				ID:             id.New(),
				WorkspaceID:    integration.WorkspaceID,
				IntegrationID:  integration.ID,
				Name:           project.Name,
				Slug:           project.PathWithNS,
				URL:            project.WebURL,
				ExternalRepoID: externalID,
				IsEnabled:      true,
				DefaultBranch:  repoDefaults,
			}
			if desc := strings.TrimSpace(project.Description); desc != "" {
				repo.Description = &desc
			}
			if err := s.repoStore.Create(ctx, repo); err != nil {
				errs = append(errs, fmt.Sprintf("project %s: storing repository: %v", project.PathWithNS, err))
			} else {
				repositoriesAdded++
			}
		}

		if _, ok := webhookConfigs[externalID]; ok {
			continue
		}

		hook, _, hookErr := client.Projects.AddProjectHook(project.ID, &gitlab.AddProjectHookOptions{
			URL:                   gitlab.Ptr(webhookURL),
			Name:                  gitlab.Ptr(webhookName),
			Description:           gitlab.Ptr(webhookDescription),
			IssuesEvents:          gitlab.Ptr(true),
			MergeRequestsEvents:   gitlab.Ptr(true),
			NoteEvents:            gitlab.Ptr(true),
			WikiPageEvents:        gitlab.Ptr(true),
			PushEvents:            gitlab.Ptr(true),
			Token:                 gitlab.Ptr(webhookSecret),
			EnableSSLVerification: gitlab.Ptr(true),
		}, gitlab.WithContext(ctx))
		if hookErr != nil {
			errs = append(errs, fmt.Sprintf("project %s: %v", project.PathWithNS, hookErr))
			continue
		}

		webhooksCreated++

		var hookID int64
		if hook != nil {
			hookID = hook.ID
		}

		value, err := json.Marshal(webhookConfigValue{
			WebhookID:   hookID,
			ProjectPath: project.PathWithNS,
			Events:      events,
		})
		if err != nil {
			errs = append(errs, fmt.Sprintf("project %s: serializing config: %v", project.PathWithNS, err))
			continue
		}

		if err := s.txRunner.WithTx(ctx, func(stores StoreProvider) error {
			config := &model.IntegrationConfig{
				ID:            id.New(),
				IntegrationID: integration.ID,
				Key:           externalID,
				Value:         value,
				ConfigType:    "webhook",
			}
			if err := stores.IntegrationConfigs().Create(ctx, config); err != nil {
				return fmt.Errorf("storing config: %w", err)
			}
			return nil
		}); err != nil {
			errs = append(errs, fmt.Sprintf("project %s: storing config: %v", project.PathWithNS, err))
		}
	}

	stateValue, err := json.Marshal(struct {
		UpdatedAt         time.Time `json:"updated_at"`
		Errors            []string  `json:"errors"`
		WebhooksCreated   int       `json:"webhooks_created"`
		RepositoriesAdded int       `json:"repositories_added"`
	}{
		WebhooksCreated:   webhooksCreated,
		RepositoriesAdded: repositoriesAdded,
		Errors:            errs,
		UpdatedAt:         time.Now().UTC(),
	})
	if err != nil {
		return nil, fmt.Errorf("serializing sync state: %w", err)
	}

	if err := s.txRunner.WithTx(ctx, func(stores StoreProvider) error {
		cfgStore := stores.IntegrationConfigs()
		existing, err := cfgStore.GetByIntegrationAndKey(ctx, integration.ID, "gitlab_sync_status")
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("fetching sync status: %w", err)
		}

		if err == nil {
			existing.Value = stateValue
			existing.ConfigType = "state"
			return cfgStore.Update(ctx, existing)
		}

		config := &model.IntegrationConfig{
			ID:            id.New(),
			IntegrationID: integration.ID,
			Key:           "gitlab_sync_status",
			Value:         stateValue,
			ConfigType:    "state",
		}
		return cfgStore.Create(ctx, config)
	}); err != nil {
		return nil, err
	}

	return &EnableRepositoriesResult{
		IntegrationID:     integration.ID,
		Projects:          selectedProjects,
		RepositoriesAdded: repositoriesAdded,
		WebhooksCreated:   webhooksCreated,
		Errors:            errs,
	}, nil
}

func (s *gitLabService) Status(ctx context.Context, workspaceID int64) (*StatusResult, error) {
	integration, err := s.stores.Integrations().GetByWorkspaceAndProvider(ctx, workspaceID, model.ProviderGitLab)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return &StatusResult{Connected: false}, nil
		}
		return nil, fmt.Errorf("fetching integration: %w", err)
	}

	var (
		webhooksCreated   int
		repositoriesAdded int
		errorsList        []string
		updatedAt         *time.Time
	)

	config, err := s.stores.IntegrationConfigs().GetByIntegrationAndKey(ctx, integration.ID, "gitlab_sync_status")
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("fetching sync status: %w", err)
	}
	if err == nil {
		var state struct {
			UpdatedAt         *time.Time `json:"updated_at"`
			Errors            []string   `json:"errors"`
			WebhooksCreated   int        `json:"webhooks_created"`
			RepositoriesAdded int        `json:"repositories_added"`
		}
		if err := json.Unmarshal(config.Value, &state); err == nil {
			webhooksCreated = state.WebhooksCreated
			repositoriesAdded = state.RepositoriesAdded
			errorsList = state.Errors
			updatedAt = state.UpdatedAt
		}
	}

	repos, err := s.repoStore.ListByIntegration(ctx, integration.ID)
	if err != nil {
		return nil, fmt.Errorf("listing repositories: %w", err)
	}

	return &StatusResult{
		Connected:         true,
		IntegrationID:     &integration.ID,
		Synced:            len(repos) > 0,
		WebhooksCreated:   webhooksCreated,
		RepositoriesAdded: repositoriesAdded,
		Errors:            errorsList,
		ReposCount:        len(repos),
		UpdatedAt:         updatedAt,
	}, nil
}

func (s *gitLabService) RefreshIntegration(ctx context.Context, workspaceID int64, webhookBaseURL string) (*SetupResult, error) {
	integration, err := s.stores.Integrations().GetByWorkspaceAndProvider(ctx, workspaceID, model.ProviderGitLab)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("integration not found for workspace")
		}
		return nil, fmt.Errorf("fetching integration: %w", err)
	}

	primary, err := s.stores.IntegrationCredentials().GetPrimaryByIntegration(ctx, integration.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("primary credential not found")
		}
		return nil, fmt.Errorf("fetching credential: %w", err)
	}

	instanceURL := ""
	if integration.ProviderBaseURL != nil {
		instanceURL = *integration.ProviderBaseURL
	}

	s.updateExistingWebhooksWithWikiEvents(ctx, integration.ID, instanceURL, primary.AccessToken)

	setupResult, err := s.SetupIntegration(ctx, SetupIntegrationParams{
		InstanceURL:    instanceURL,
		Token:          primary.AccessToken,
		WorkspaceID:    integration.WorkspaceID,
		OrganizationID: integration.OrganizationID,
		SetupByUserID:  integration.SetupByUserID,
		WebhookBaseURL: webhookBaseURL,
	})
	if err != nil {
		return nil, err
	}

	repos, err := s.repoStore.ListEnabledByIntegration(ctx, integration.ID)
	if err != nil {
		return nil, fmt.Errorf("listing enabled repositories: %w", err)
	}

	projectIDs := make([]int64, 0, len(repos))
	for _, repo := range repos {
		projectID, err := strconv.ParseInt(repo.ExternalRepoID, 10, 64)
		if err != nil {
			continue
		}
		projectIDs = append(projectIDs, projectID)
	}

	if len(projectIDs) > 0 {
		if _, err := s.EnableRepositories(ctx, EnableRepositoriesParams{
			WorkspaceID:    workspaceID,
			ProjectIDs:     projectIDs,
			WebhookBaseURL: webhookBaseURL,
		}); err != nil {
			return nil, err
		}
	}

	return setupResult, nil
}

func (s *gitLabService) ListEnabledProjectIDs(ctx context.Context, workspaceID int64) ([]int64, error) {
	integration, err := s.stores.Integrations().GetByWorkspaceAndProvider(ctx, workspaceID, model.ProviderGitLab)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return []int64{}, nil
		}
		return nil, err
	}

	repos, err := s.repoStore.ListEnabledByIntegration(ctx, integration.ID)
	if err != nil {
		return nil, fmt.Errorf("listing enabled repositories: %w", err)
	}

	projectIDs := make([]int64, 0, len(repos))
	for _, repo := range repos {
		projectID, err := strconv.ParseInt(repo.ExternalRepoID, 10, 64)
		if err != nil {
			continue
		}
		projectIDs = append(projectIDs, projectID)
	}

	return projectIDs, nil
}

func (s *gitLabService) updateExistingWebhooksWithWikiEvents(ctx context.Context, integrationID int64, instanceURL, token string) {
	webhookName := "Relay"
	webhookDescription := "Relay outbound webhook"
	configs, err := s.stores.IntegrationConfigs().ListByIntegrationAndType(ctx, integrationID, "webhook")
	if err != nil {
		slog.WarnContext(ctx, "failed to list webhook configs for wiki update",
			"integration_id", integrationID,
			"error", err,
		)
		return
	}

	if len(configs) == 0 {
		return
	}

	client, err := s.newClient(instanceURL, token)
	if err != nil {
		slog.WarnContext(ctx, "failed to create gitlab client for wiki update",
			"integration_id", integrationID,
			"error", err,
		)
		return
	}

	wikiEvent := "wiki_page_events"
	for _, config := range configs {
		var cfg webhookConfigValue
		if err := json.Unmarshal(config.Value, &cfg); err != nil {
			slog.WarnContext(ctx, "failed to unmarshal webhook config",
				"config_id", config.ID,
				"error", err,
			)
			continue
		}

		if cfg.WebhookID == 0 {
			continue
		}

		hasWikiEvents := false
		for _, event := range cfg.Events {
			if event == wikiEvent {
				hasWikiEvents = true
				break
			}
		}
		if hasWikiEvents {
			continue
		}

		projectID, err := strconv.ParseInt(config.Key, 10, 64)
		if err != nil {
			slog.WarnContext(ctx, "failed to parse project id from config key",
				"config_key", config.Key,
				"error", err,
			)
			continue
		}

		_, _, editErr := client.Projects.EditProjectHook(projectID, cfg.WebhookID, &gitlab.EditProjectHookOptions{
			Name:           gitlab.Ptr(webhookName),
			Description:    gitlab.Ptr(webhookDescription),
			WikiPageEvents: gitlab.Ptr(true),
		}, gitlab.WithContext(ctx))
		if editErr != nil {
			slog.WarnContext(ctx, "failed to update webhook with wiki events",
				"project_id", projectID,
				"webhook_id", cfg.WebhookID,
				"error", editErr,
			)
			continue
		}

		cfg.Events = append(cfg.Events, wikiEvent)
		updatedValue, err := json.Marshal(cfg)
		if err != nil {
			slog.WarnContext(ctx, "failed to marshal updated config",
				"config_id", config.ID,
				"error", err,
			)
			continue
		}
		config.Value = updatedValue

		if err := s.stores.IntegrationConfigs().Update(ctx, &config); err != nil {
			slog.WarnContext(ctx, "failed to update config with wiki events",
				"config_id", config.ID,
				"error", err,
			)
			continue
		}

		slog.InfoContext(ctx, "updated webhook with wiki events",
			"project_id", projectID,
			"webhook_id", cfg.WebhookID,
		)
	}
}

func (s *gitLabService) newClient(instanceURL, token string) (*gitlab.Client, error) {
	baseURL := strings.TrimSuffix(instanceURL, "/") + "/api/v4"
	return gitlab.NewClient(
		token,
		gitlab.WithBaseURL(baseURL),
	)
}

func generateSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func ensureWebhookSecret(ctx context.Context, credentialStore store.IntegrationCredentialStore, integrationID int64) (string, error) {
	creds, err := credentialStore.ListActiveByIntegration(ctx, integrationID)
	if err != nil {
		return "", err
	}
	for _, cred := range creds {
		if cred.CredentialType == model.CredentialTypeWebhookSecret {
			return cred.AccessToken, nil
		}
	}

	secret, err := generateSecret()
	if err != nil {
		return "", err
	}

	secretCred := &model.IntegrationCredential{
		ID:             id.New(),
		IntegrationID:  integrationID,
		CredentialType: model.CredentialTypeWebhookSecret,
		AccessToken:    secret,
		IsPrimary:      false,
	}

	if err := credentialStore.Create(ctx, secretCred); err != nil {
		return "", err
	}

	return secret, nil
}
