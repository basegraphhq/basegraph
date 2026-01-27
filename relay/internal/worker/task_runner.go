package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"basegraph.co/relay/common/id"
	"basegraph.co/relay/common/logger"
	"basegraph.co/relay/internal/model"
	"basegraph.co/relay/internal/store"
	"github.com/redis/go-redis/v9"
)

const (
	deployKeyConfigType         = "deploy_key"
	deployKeyWorkspaceConfigKey = "deploy_key_workspace"
	deployKeyWorkspaceType      = "deploy_key_workspace"

	statusStreamMaxLen = 2000
)

var (
	ErrRepoNotReady          = errors.New("repo not ready")
	ErrDefaultBranchRequired = errors.New("default branch required")
)

type TaskRunnerConfig struct {
	WorkspaceID int64
	DataDir     string
	Stores      *store.Stores
	Redis       *redis.Client
	Runner      CommandRunner
}

type TaskRunner struct {
	stores       *store.Stores
	redis        *redis.Client
	runner       CommandRunner
	workspace    *model.Workspace
	org          *model.Organization
	dataDir      string
	repoRoot     string
	statusStream string
}

func NewTaskRunner(ctx context.Context, cfg TaskRunnerConfig) (*TaskRunner, error) {
	runner := cfg.Runner
	if runner == nil {
		runner = ExecCommandRunner{}
	}

	workspace, err := cfg.Stores.Workspaces().GetByID(ctx, cfg.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("fetching workspace: %w", err)
	}

	org, err := cfg.Stores.Organizations().GetByID(ctx, workspace.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("fetching organization: %w", err)
	}

	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = "/data"
	}

	repoRoot := filepath.Join(dataDir, org.Slug)
	statusStream := fmt.Sprintf("agent-status:org-%d:workspace-%d", org.ID, workspace.ID)

	return &TaskRunner{
		stores:       cfg.Stores,
		redis:        cfg.Redis,
		runner:       runner,
		workspace:    workspace,
		org:          org,
		dataDir:      dataDir,
		repoRoot:     repoRoot,
		statusStream: statusStream,
	}, nil
}

func (r *TaskRunner) RepoRoot() string {
	return r.repoRoot
}

func (r *TaskRunner) Workspace() *model.Workspace {
	return r.workspace
}

func (r *TaskRunner) Organization() *model.Organization {
	return r.org
}

func (r *TaskRunner) EnsureIssueReady(ctx context.Context, issueID int64) (bool, string, error) {
	issue, err := r.stores.Issues().GetByID(ctx, issueID)
	if err != nil {
		return false, "", fmt.Errorf("fetching issue: %w", err)
	}

	integration, err := r.stores.Integrations().GetByID(ctx, issue.IntegrationID)
	if err != nil {
		return false, "", fmt.Errorf("fetching integration: %w", err)
	}

	workspace, err := r.stores.Workspaces().GetByID(ctx, integration.WorkspaceID)
	if err != nil {
		return false, "", fmt.Errorf("fetching workspace: %w", err)
	}

	if workspace.RepoReadyAt == nil {
		return false, "workspace_not_ready", nil
	}

	if issue.ExternalProjectID == nil || strings.TrimSpace(*issue.ExternalProjectID) == "" {
		return false, "missing_external_project_id", nil
	}

	repo, err := r.stores.Repos().GetByExternalID(ctx, integration.ID, *issue.ExternalProjectID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return false, "repo_not_found", nil
		}
		return false, "", fmt.Errorf("fetching repository: %w", err)
	}

	if !repo.IsEnabled {
		return false, "repo_disabled", nil
	}

	repoPath := filepath.Join(r.repoRoot, repo.Slug)
	if !isDir(repoPath) {
		return false, "repo_missing", nil
	}

	return true, "", nil
}

func (r *TaskRunner) HandleWorkspaceSetup(ctx context.Context, runID int64) error {
	ctx = logger.WithLogFields(ctx, logger.LogFields{Component: "relay.worker.workspace_setup"})

	r.emitStatus(ctx, runID, "info", "starting", "workspace setup started", nil)

	statusUpdated := false
	defer func() {
		if !statusUpdated && ctx.Err() == nil {
			msg := "workspace setup failed"
			finishedAt := time.Now().UTC()
			_ = r.updateRunStatus(ctx, runID, model.WorkspaceEventStatusFailed, &msg, nil, nil, &finishedAt)
		}
	}()

	startedAt := time.Now().UTC()
	if err := r.updateRunStatus(ctx, runID, model.WorkspaceEventStatusRunning, nil, nil, &startedAt, nil); err != nil {
		return err
	}

	if err := os.MkdirAll(r.repoRoot, 0o755); err != nil {
		r.emitStatus(ctx, runID, "error", "mkdir", "failed to create repo root", map[string]any{"error": err.Error()})
		return fmt.Errorf("creating repo root: %w", err)
	}

	integration, credential, err := r.loadCodeRepoIntegration(ctx)
	if err != nil {
		return err
	}

	provider, err := newRepoProvider(integration, credential, r.stores.IntegrationConfigs())
	if err != nil {
		return err
	}

	repos, err := r.stores.Repos().ListEnabledByIntegration(ctx, integration.ID)
	if err != nil {
		return fmt.Errorf("listing enabled repositories: %w", err)
	}

	allRepos, err := r.stores.Repos().ListByIntegration(ctx, integration.ID)
	if err != nil {
		return fmt.Errorf("listing repositories: %w", err)
	}

	for _, repo := range allRepos {
		if repo.IsEnabled {
			continue
		}
		path := filepath.Join(r.repoRoot, repo.Slug)
		if isDir(path) {
			r.emitStatus(ctx, runID, "info", "delete", "removing disabled repository", map[string]any{"repo": repo.Slug})
			if err := os.RemoveAll(path); err != nil {
				r.emitStatus(ctx, runID, "error", "delete", "failed to remove repository", map[string]any{"repo": repo.Slug, "error": err.Error()})
			}
		}
	}

	if len(repos) == 0 {
		r.emitStatus(ctx, runID, "info", "empty", "no enabled repositories", nil)
		finishedAt := time.Now().UTC()
		if err := r.updateRunStatus(ctx, runID, model.WorkspaceEventStatusSucceeded, nil, nil, nil, &finishedAt); err != nil {
			return err
		}
		return nil
	}

	sshDir := filepath.Join(r.repoRoot, ".ssh")
	keyPath := filepath.Join(sshDir, fmt.Sprintf("id_relay_ws_%d", r.workspace.ID))
	pubPath := keyPath + ".pub"

	if err := r.ensureSSHKey(ctx, sshDir, keyPath, pubPath, fmt.Sprintf("relay-ws-%d", r.workspace.ID)); err != nil {
		return err
	}

	publicKey, err := os.ReadFile(pubPath)
	if err != nil {
		return fmt.Errorf("reading public key: %w", err)
	}
	keySpec := DeployKeySpec{
		Title:     fmt.Sprintf("relay-ws-%d", r.workspace.ID),
		PublicKey: strings.TrimSpace(string(publicKey)),
		CanPush:   true,
	}

	var (
		failed    []string
		succeeded int
	)

	for _, repo := range repos {
		r.emitStatus(ctx, runID, "info", "repo", "syncing repository", map[string]any{"repo": repo.Slug})
		cloneURL, err := provider.CloneURL(repo)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: resolve clone url: %v", repo.Slug, err))
			continue
		}

		host, err := sshHostFromCloneURL(cloneURL)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: parse host: %v", repo.Slug, err))
			continue
		}

		knownHostsPath := filepath.Join(sshDir, "known_hosts")
		if err := r.ensureKnownHost(ctx, host, knownHostsPath); err != nil {
			failed = append(failed, fmt.Sprintf("%s: known_hosts: %v", repo.Slug, err))
			continue
		}

		if _, err := provider.EnsureDeployKey(ctx, repo, keySpec); err != nil {
			r.emitStatus(ctx, runID, "error", "deploy_key", "failed to ensure deploy key", map[string]any{"repo": repo.Slug, "error": err.Error()})
			failed = append(failed, fmt.Sprintf("%s: deploy key: %v", repo.Slug, err))
			continue
		}

		repoPath := filepath.Join(r.repoRoot, repo.Slug)
		if isDir(filepath.Join(repoPath, ".git")) {
			r.emitStatus(ctx, runID, "info", "repo_exists", "repository already cloned", map[string]any{"repo": repo.Slug})
			succeeded++
			continue
		}

		if err := os.MkdirAll(filepath.Dir(repoPath), 0o755); err != nil {
			failed = append(failed, fmt.Sprintf("%s: mkdir: %v", repo.Slug, err))
			continue
		}

		args := []string{"clone", "--depth", "1"}
		if repo.DefaultBranch != nil && strings.TrimSpace(*repo.DefaultBranch) != "" {
			args = append(args, "--branch", *repo.DefaultBranch)
		}
		args = append(args, cloneURL, repoPath)

		if err := r.runGit(ctx, "", keyPath, knownHostsPath, args...); err != nil {
			r.emitStatus(ctx, runID, "error", "clone", "repository clone failed", map[string]any{"repo": repo.Slug, "error": err.Error()})
			failed = append(failed, fmt.Sprintf("%s: clone: %v", repo.Slug, err))
			continue
		}

		r.emitStatus(ctx, runID, "info", "clone", "repository cloned", map[string]any{"repo": repo.Slug})

		succeeded++
	}

	if succeeded > 0 {
		if _, err := r.stores.Workspaces().SetRepoReadyAt(ctx, r.workspace.ID, time.Now().UTC()); err != nil {
			return fmt.Errorf("setting repo ready: %w", err)
		}
	}

	status := model.WorkspaceEventStatusSucceeded
	if len(failed) > 0 && succeeded > 0 {
		status = model.WorkspaceEventStatusSucceededWithErrors
	}
	if succeeded == 0 && len(failed) > 0 {
		status = model.WorkspaceEventStatusFailed
	}

	metadata, err := json.Marshal(map[string]any{
		"failed_repos": failed,
		"succeeded":    succeeded,
	})
	if err != nil {
		return fmt.Errorf("serializing setup metadata: %w", err)
	}

	var errMsg *string
	if status == model.WorkspaceEventStatusFailed && len(failed) > 0 {
		msg := failed[0]
		errMsg = &msg
	}

	finishedAt := time.Now().UTC()
	if err := r.updateRunStatus(ctx, runID, status, errMsg, metadata, nil, &finishedAt); err != nil {
		return err
	}
	statusUpdated = true

	r.emitStatus(ctx, runID, "info", "done", "workspace setup completed", map[string]any{"status": status})

	if status == model.WorkspaceEventStatusFailed {
		return fmt.Errorf("workspace setup failed: %s", *errMsg)
	}

	return nil
}

func (r *TaskRunner) HandleRepoSync(ctx context.Context, runID int64, repoID int64, branch string) error {
	ctx = logger.WithLogFields(ctx, logger.LogFields{Component: "relay.worker.repo_sync"})

	r.emitStatus(ctx, runID, "info", "starting", "repo sync started", map[string]any{"repo_id": repoID})

	statusUpdated := false
	defer func() {
		if !statusUpdated && ctx.Err() == nil {
			msg := "repo sync failed"
			finishedAt := time.Now().UTC()
			_ = r.updateRunStatus(ctx, runID, model.WorkspaceEventStatusFailed, &msg, nil, nil, &finishedAt)
		}
	}()

	startedAt := time.Now().UTC()
	if err := r.updateRunStatus(ctx, runID, model.WorkspaceEventStatusRunning, nil, nil, &startedAt, nil); err != nil {
		return err
	}

	repo, err := r.stores.Repos().GetByID(ctx, repoID)
	if err != nil {
		return fmt.Errorf("fetching repo: %w", err)
	}

	if repo.DefaultBranch == nil || strings.TrimSpace(*repo.DefaultBranch) == "" {
		r.emitStatus(ctx, runID, "error", "branch", "default branch missing", map[string]any{"repo": repo.Slug})
		return ErrDefaultBranchRequired
	}

	if branch == "" {
		branch = *repo.DefaultBranch
	}

	if branch != *repo.DefaultBranch {
		return ErrDefaultBranchRequired
	}

	repoPath := filepath.Join(r.repoRoot, repo.Slug)
	if !isDir(filepath.Join(repoPath, ".git")) {
		r.emitStatus(ctx, runID, "error", "repo_missing", "repository not cloned", map[string]any{"repo": repo.Slug})
		return ErrRepoNotReady
	}

	integration, credential, err := r.loadCodeRepoIntegration(ctx)
	if err != nil {
		return err
	}

	provider, err := newRepoProvider(integration, credential, r.stores.IntegrationConfigs())
	if err != nil {
		return err
	}

	cloneURL, err := provider.CloneURL(*repo)
	if err != nil {
		return err
	}

	host, err := sshHostFromCloneURL(cloneURL)
	if err != nil {
		return err
	}

	sshDir := filepath.Join(r.repoRoot, ".ssh")
	keyPath := filepath.Join(sshDir, fmt.Sprintf("id_relay_ws_%d", r.workspace.ID))
	pubPath := keyPath + ".pub"
	if err := r.ensureSSHKey(ctx, sshDir, keyPath, pubPath, fmt.Sprintf("relay-ws-%d", r.workspace.ID)); err != nil {
		return err
	}

	knownHostsPath := filepath.Join(sshDir, "known_hosts")
	if err := r.ensureKnownHost(ctx, host, knownHostsPath); err != nil {
		return err
	}

	if err := r.runGit(ctx, repoPath, keyPath, knownHostsPath, "fetch", "origin", branch); err != nil {
		r.emitStatus(ctx, runID, "error", "fetch", "git fetch failed", map[string]any{"repo": repo.Slug, "error": err.Error()})
		return err
	}
	if err := r.runGit(ctx, repoPath, keyPath, knownHostsPath, "reset", "--hard", fmt.Sprintf("origin/%s", branch)); err != nil {
		r.emitStatus(ctx, runID, "error", "reset", "git reset failed", map[string]any{"repo": repo.Slug, "error": err.Error()})
		return err
	}

	finishedAt := time.Now().UTC()
	if err := r.updateRunStatus(ctx, runID, model.WorkspaceEventStatusSucceeded, nil, nil, nil, &finishedAt); err != nil {
		return err
	}
	statusUpdated = true

	r.emitStatus(ctx, runID, "info", "done", "repo sync completed", map[string]any{"repo": repo.Slug, "branch": branch})

	return nil
}

func (r *TaskRunner) loadCodeRepoIntegration(ctx context.Context) (*model.Integration, *model.IntegrationCredential, error) {
	integrations, err := r.stores.Integrations().ListByCapability(ctx, r.workspace.ID, model.CapabilityCodeRepo)
	if err != nil {
		return nil, nil, fmt.Errorf("listing integrations: %w", err)
	}
	if len(integrations) == 0 {
		return nil, nil, fmt.Errorf("no code repo integrations configured")
	}
	if len(integrations) > 1 {
		return nil, nil, fmt.Errorf("multiple code repo integrations not supported")
	}

	integration := integrations[0]
	credential, err := r.stores.IntegrationCredentials().GetPrimaryByIntegration(ctx, integration.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("fetching primary credential: %w", err)
	}
	return &integration, credential, nil
}

func (r *TaskRunner) updateRunStatus(ctx context.Context, runID int64, status model.WorkspaceEventStatus, errMsg *string, metadata json.RawMessage, startedAt *time.Time, finishedAt *time.Time) error {
	log := &model.WorkspaceEventLog{
		ID:       runID,
		Status:   string(status),
		Error:    errMsg,
		Metadata: metadata,
	}
	_, err := r.stores.WorkspaceEventLogs().Update(ctx, log, startedAt, finishedAt)
	if err != nil {
		return fmt.Errorf("updating run status: %w", err)
	}
	return nil
}

func (r *TaskRunner) runGit(ctx context.Context, dir string, keyPath string, knownHostsPath string, args ...string) error {
	sshCmd := fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=yes -o UserKnownHostsFile=%s", keyPath, knownHostsPath)
	output, err := r.runner.Run(ctx, Command{
		Name: "git",
		Args: args,
		Dir:  dir,
		Env: []string{
			"GIT_TERMINAL_PROMPT=0",
			"GIT_SSH_COMMAND=" + sshCmd,
		},
	})
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, string(output))
	}
	return nil
}

func (r *TaskRunner) emitStatus(ctx context.Context, runID int64, level string, step string, message string, fields map[string]any) {
	if r.redis == nil {
		return
	}
	values := map[string]any{
		"run_id":  runID,
		"level":   level,
		"step":    step,
		"message": message,
		"ts":      time.Now().UTC().Format(time.RFC3339Nano),
	}
	for key, value := range fields {
		values[key] = value
	}
	_ = r.redis.XAdd(ctx, &redis.XAddArgs{
		Stream: r.statusStream,
		MaxLen: statusStreamMaxLen,
		Approx: true,
		Values: values,
	}).Err()
}

func (r *TaskRunner) ensureSSHKey(ctx context.Context, sshDir string, keyPath string, pubPath string, comment string) error {
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return fmt.Errorf("creating ssh dir: %w", err)
	}

	if _, err := os.Stat(keyPath); err == nil {
		if _, err := os.Stat(pubPath); err == nil {
			return nil
		}
	}

	output, err := r.runner.Run(ctx, Command{
		Name: "ssh-keygen",
		Args: []string{"-t", "ed25519", "-N", "", "-f", keyPath, "-C", comment},
	})
	if err != nil {
		return fmt.Errorf("ssh-keygen failed: %w: %s", err, string(output))
	}
	return nil
}

func (r *TaskRunner) ensureKnownHost(ctx context.Context, host string, knownHostsPath string) error {
	if host == "" {
		return fmt.Errorf("host is required")
	}

	if hasKnownHost(knownHostsPath, host) {
		return nil
	}

	output, err := r.runner.Run(ctx, Command{
		Name: "ssh-keyscan",
		Args: []string{"-t", "rsa,ed25519", host},
	})
	if err != nil {
		return fmt.Errorf("ssh-keyscan failed: %w", err)
	}

	file, err := os.OpenFile(knownHostsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("opening known_hosts: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(output); err != nil {
		return fmt.Errorf("writing known_hosts: %w", err)
	}
	return nil
}

func hasKnownHost(path string, host string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), host)
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func sshHostFromCloneURL(cloneURL string) (string, error) {
	parts := strings.Split(cloneURL, "@")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid clone url")
	}
	hostPart := parts[1]
	idx := strings.Index(hostPart, ":")
	if idx == -1 {
		return "", fmt.Errorf("invalid clone url host")
	}
	return hostPart[:idx], nil
}

type DeployKeySpec struct {
	Title     string
	PublicKey string
	CanPush   bool
}

type RepoProvider interface {
	EnsureDeployKey(ctx context.Context, repo model.Repository, key DeployKeySpec) (int64, error)
	CloneURL(repo model.Repository) (string, error)
}

func newRepoProvider(integration *model.Integration, credential *model.IntegrationCredential, configs store.IntegrationConfigStore) (RepoProvider, error) {
	switch integration.Provider {
	case model.ProviderGitLab:
		return newGitLabRepoProvider(integration, credential, configs)
	default:
		return nil, fmt.Errorf("unsupported provider %s", integration.Provider)
	}
}

type deployKeyConfig struct {
	DeployKeyID int64  `json:"deploy_key_id"`
	Title       string `json:"title"`
	PublicKey   string `json:"public_key"`
	CanPush     bool   `json:"can_push"`
}

func buildDeployKeyConfig(id int64, key DeployKeySpec) deployKeyConfig {
	return deployKeyConfig{
		DeployKeyID: id,
		Title:       key.Title,
		PublicKey:   key.PublicKey,
		CanPush:     key.CanPush,
	}
}

func saveIntegrationConfig(ctx context.Context, configStore store.IntegrationConfigStore, integrationID int64, key string, configType string, value []byte) error {
	existing, err := configStore.GetByIntegrationAndKey(ctx, integrationID, key)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}

	if err == nil {
		existing.Value = value
		existing.ConfigType = configType
		return configStore.Update(ctx, existing)
	}

	config := &model.IntegrationConfig{
		ID:            id.New(),
		IntegrationID: integrationID,
		Key:           key,
		Value:         value,
		ConfigType:    configType,
	}
	return configStore.Create(ctx, config)
}
