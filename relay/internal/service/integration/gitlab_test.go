package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"basegraph.app/relay/common/id"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GitLabService", func() {
	var (
		ctx      context.Context
		initOnce sync.Once
	)

	BeforeEach(func() {
		ctx = context.Background()
		initOnce.Do(func() {
			Expect(id.Init(1)).To(Succeed())
		})
	})

	It("lists projects with pagination", func() {
		mock := newGitLabAPIMock()
		mock.projects = []gitlabProject{
			{ID: 1, Name: "p1", PathWithNamespace: "group/p1", WebURL: "http://git/p1"},
			{ID: 2, Name: "p2", PathWithNamespace: "group/p2", WebURL: "http://git/p2"},
			{ID: 3, Name: "p3", PathWithNamespace: "group/p3", WebURL: "http://git/p3"},
		}
		mock.start()
		defer mock.close()

		svc := &gitLabService{}

		projects, err := svc.ListProjects(ctx, mock.baseURL(), "token")
		Expect(err).NotTo(HaveOccurred())
		Expect(projects).To(HaveLen(3))
		Expect(projects[0].PathWithNS).To(Equal("group/p1"))
		Expect(projects[2].Name).To(Equal("p3"))
	})

	It("sets up integration and creates credentials and hooks", func() {
		mock := newGitLabAPIMock()
		mock.projects = []gitlabProject{
			{ID: 10, Name: "p10", PathWithNamespace: "g/p10", WebURL: "http://git/p10"},
		}
		mock.start()
		defer mock.close()

		intStore := newFakeIntegrationStore()
		credStore := newFakeCredentialStore()
		repoStore := newFakeRepoStore()
		configStore := newFakeIntegrationConfigStore()
		tx := &fakeTxRunner{provider: &fakeStoreProvider{
			integrationStore:       intStore,
			credentialStore:        credStore,
			integrationConfigStore: configStore,
			repoStore:              repoStore,
		}}

		svc := &gitLabService{
			txRunner:  tx,
			repoStore: repoStore,
		}

		result, err := svc.SetupIntegration(ctx, SetupIntegrationParams{
			InstanceURL:    mock.baseURL(),
			Token:          "pat",
			WorkspaceID:    1,
			OrganizationID: 2,
			SetupByUserID:  3,
			WebhookBaseURL: "https://relay",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.WebhooksCreated).To(Equal(1))
		Expect(result.Errors).To(BeEmpty())

		Expect(intStore.created).To(HaveLen(1))
		Expect(credStore.created).To(HaveLen(2))
		Expect(findCredentialType(credStore.created, model.CredentialTypeAPIKey)).To(BeTrue())
		Expect(findCredentialType(credStore.created, model.CredentialTypeWebhookSecret)).To(BeTrue())
		Expect(mock.hookCalls).To(Equal([]int64{10}))
	})

	It("continues when some hooks fail", func() {
		mock := newGitLabAPIMock()
		mock.projects = []gitlabProject{
			{ID: 100, Name: "ok", PathWithNamespace: "g/ok"},
			{ID: 200, Name: "fail", PathWithNamespace: "g/fail"},
		}
		mock.hookErrors[200] = http.StatusInternalServerError
		mock.start()
		defer mock.close()

		intStore := newFakeIntegrationStore()
		credStore := newFakeCredentialStore()
		repoStore := newFakeRepoStore()
		configStore := newFakeIntegrationConfigStore()
		tx := &fakeTxRunner{provider: &fakeStoreProvider{
			integrationStore:       intStore,
			credentialStore:        credStore,
			integrationConfigStore: configStore,
			repoStore:              repoStore,
		}}

		svc := &gitLabService{
			txRunner:  tx,
			repoStore: repoStore,
		}

		result, err := svc.SetupIntegration(ctx, SetupIntegrationParams{
			InstanceURL:    mock.baseURL(),
			Token:          "pat",
			WorkspaceID:    1,
			OrganizationID: 2,
			SetupByUserID:  3,
			WebhookBaseURL: "https://relay",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.WebhooksCreated).To(Equal(1))
		Expect(result.Errors).To(HaveLen(1))
		Expect(result.Errors[0]).To(ContainSubstring("g/fail"))
	})

	It("enforces concurrency limit of 5 when creating hooks", func() {
		mock := newGitLabAPIMock()
		for i := 0; i < 8; i++ {
			mock.projects = append(mock.projects, gitlabProject{
				ID:                int64(1000 + i),
				Name:              "p",
				PathWithNamespace: "g/p" + strconv.Itoa(i),
			})
		}
		mock.hookDelay = 50 * time.Millisecond
		mock.start()
		defer mock.close()

		repoStore := newFakeRepoStore()
		configStore := newFakeIntegrationConfigStore()
		tx := &fakeTxRunner{provider: &fakeStoreProvider{
			integrationStore:       newFakeIntegrationStore(),
			credentialStore:        newFakeCredentialStore(),
			integrationConfigStore: configStore,
			repoStore:              repoStore,
		}}

		svc := &gitLabService{
			txRunner:  tx,
			repoStore: repoStore,
		}

		_, err := svc.SetupIntegration(ctx, SetupIntegrationParams{
			InstanceURL:    mock.baseURL(),
			Token:          "pat",
			WorkspaceID:    1,
			OrganizationID: 2,
			SetupByUserID:  3,
			WebhookBaseURL: "https://relay",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(mock.maxConcurrentLoad()).To(BeNumerically("<=", 5))
	})

	It("errors when webhook base URL is missing", func() {
		mock := newGitLabAPIMock()
		mock.start()
		defer mock.close()

		repoStore := newFakeRepoStore()
		configStore := newFakeIntegrationConfigStore()
		tx := &fakeTxRunner{provider: &fakeStoreProvider{
			integrationStore:       newFakeIntegrationStore(),
			credentialStore:        newFakeCredentialStore(),
			integrationConfigStore: configStore,
			repoStore:              repoStore,
		}}

		svc := &gitLabService{
			txRunner:  tx,
			repoStore: repoStore,
		}

		_, err := svc.SetupIntegration(ctx, SetupIntegrationParams{
			InstanceURL:    mock.baseURL(),
			Token:          "pat",
			WorkspaceID:    1,
			OrganizationID: 2,
			SetupByUserID:  3,
			WebhookBaseURL: "",
		})
		Expect(err).To(HaveOccurred())
	})

	It("includes wiki_page_events when creating new webhooks", func() {
		mock := newGitLabAPIMock()
		mock.projects = []gitlabProject{
			{ID: 10, Name: "p10", PathWithNamespace: "g/p10", WebURL: "http://git/p10"},
		}
		mock.start()
		defer mock.close()

		intStore := newFakeIntegrationStore()
		credStore := newFakeCredentialStore()
		repoStore := newFakeRepoStore()
		configStore := newFakeIntegrationConfigStore()
		tx := &fakeTxRunner{provider: &fakeStoreProvider{
			integrationStore:       intStore,
			credentialStore:        credStore,
			integrationConfigStore: configStore,
			repoStore:              repoStore,
		}}

		svc := &gitLabService{
			txRunner:  tx,
			repoStore: repoStore,
		}

		result, err := svc.SetupIntegration(ctx, SetupIntegrationParams{
			InstanceURL:    mock.baseURL(),
			Token:          "pat",
			WorkspaceID:    1,
			OrganizationID: 2,
			SetupByUserID:  3,
			WebhookBaseURL: "https://relay",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.WebhooksCreated).To(Equal(1))

		var webhookConfig *model.IntegrationConfig
		for i := range configStore.created {
			if configStore.created[i].ConfigType == "webhook" {
				webhookConfig = &configStore.created[i]
				break
			}
		}
		Expect(webhookConfig).NotTo(BeNil())

		var cfg webhookConfigValue
		err = json.Unmarshal(webhookConfig.Value, &cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Events).To(ContainElement("wiki_page_events"))
	})
})

// --- test fixtures ---

type gitlabProject struct {
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
	WebURL            string `json:"web_url,omitempty"`
	Description       string `json:"description,omitempty"`
	ID                int64  `json:"id"`
}

type gitlabAPIMock struct {
	hookErrors    map[int64]int
	server        *httptest.Server
	projects      []gitlabProject
	hookCalls     []int64
	editHookCalls []editHookCall
	hookDelay     time.Duration
	hookMu        sync.Mutex
	inFlight      int32
	maxInFlight   int32
}

type editHookCall struct {
	ProjectID      int64
	HookID         int64
	WikiPageEvents bool
}

func newGitLabAPIMock() *gitlabAPIMock {
	return &gitlabAPIMock{
		hookErrors: make(map[int64]int),
	}
}

func (m *gitlabAPIMock) baseURL() string {
	return strings.TrimSuffix(m.server.URL, "")
}

func (m *gitlabAPIMock) start() {
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v4/projects") && r.Method == http.MethodGet:
			m.handleListProjects(w, r)
		case strings.HasPrefix(r.URL.Path, "/api/v4/projects/") && strings.HasSuffix(r.URL.Path, "/hooks") && r.Method == http.MethodPost:
			m.handleAddHook(w, r)
		case strings.HasPrefix(r.URL.Path, "/api/v4/projects/") && strings.Contains(r.URL.Path, "/hooks/") && r.Method == http.MethodPut:
			m.handleEditHook(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
}

func (m *gitlabAPIMock) close() {
	m.server.Close()
}

func (m *gitlabAPIMock) handleListProjects(w http.ResponseWriter, r *http.Request) {
	page := r.URL.Query().Get("page")
	per := r.URL.Query().Get("per_page")
	pageNum, _ := strconv.Atoi(page)
	perPage, _ := strconv.Atoi(per)
	if pageNum == 0 {
		pageNum = 1
	}
	if perPage == 0 {
		perPage = 100
	}

	start := (pageNum - 1) * perPage
	if start >= len(m.projects) {
		json.NewEncoder(w).Encode([]gitlabProject{})
		return
	}
	end := start + perPage
	if end > len(m.projects) {
		end = len(m.projects)
	}
	nextPage := 0
	if end < len(m.projects) {
		nextPage = pageNum + 1
	}
	if nextPage > 0 {
		w.Header().Set("X-Next-Page", strconv.Itoa(nextPage))
	}
	_ = json.NewEncoder(w).Encode(m.projects[start:end])
}

func (m *gitlabAPIMock) handleAddHook(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	projectIDStr := parts[len(parts)-2]
	projectID, _ := strconv.ParseInt(projectIDStr, 10, 64)

	current := atomic.AddInt32(&m.inFlight, 1)
	defer atomic.AddInt32(&m.inFlight, -1)

	for {
		prev := atomic.LoadInt32(&m.maxInFlight)
		if current <= prev || atomic.CompareAndSwapInt32(&m.maxInFlight, prev, current) {
			break
		}
	}

	if m.hookDelay > 0 {
		time.Sleep(m.hookDelay)
	}

	if status, ok := m.hookErrors[projectID]; ok {
		http.Error(w, "hook error", status)
		return
	}

	m.hookMu.Lock()
	m.hookCalls = append(m.hookCalls, projectID)
	m.hookMu.Unlock()

	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"id":123}`))
}

func (m *gitlabAPIMock) handleEditHook(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 6 {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	projectIDStr := parts[len(parts)-3]
	hookIDStr := parts[len(parts)-1]
	projectID, _ := strconv.ParseInt(projectIDStr, 10, 64)
	hookID, _ := strconv.ParseInt(hookIDStr, 10, 64)

	var body struct {
		WikiPageEvents *bool `json:"wiki_page_events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}

	m.hookMu.Lock()
	m.editHookCalls = append(m.editHookCalls, editHookCall{
		ProjectID:      projectID,
		HookID:         hookID,
		WikiPageEvents: body.WikiPageEvents != nil && *body.WikiPageEvents,
	})
	m.hookMu.Unlock()

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"id":` + hookIDStr + `}`))
}

func (m *gitlabAPIMock) maxConcurrentLoad() int32 {
	return atomic.LoadInt32(&m.maxInFlight)
}

type fakeIntegrationStore struct {
	existing *model.Integration
	created  []*model.Integration
	mu       sync.Mutex
}

func newFakeIntegrationStore() *fakeIntegrationStore {
	return &fakeIntegrationStore{}
}

func (f *fakeIntegrationStore) GetByID(ctx context.Context, id int64) (*model.Integration, error) {
	if f.existing != nil && f.existing.ID == id {
		return f.existing, nil
	}
	return nil, store.ErrNotFound
}

func (f *fakeIntegrationStore) GetByWorkspaceAndProvider(ctx context.Context, workspaceID int64, provider model.Provider) (*model.Integration, error) {
	if f.existing != nil && f.existing.WorkspaceID == workspaceID && f.existing.Provider == provider {
		return f.existing, nil
	}
	return nil, store.ErrNotFound
}

func (f *fakeIntegrationStore) Create(ctx context.Context, integration *model.Integration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	copy := *integration
	f.created = append(f.created, &copy)
	return nil
}

func (f *fakeIntegrationStore) Update(ctx context.Context, integration *model.Integration) error {
	return nil
}

func (f *fakeIntegrationStore) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	return nil
}

func (f *fakeIntegrationStore) Delete(ctx context.Context, id int64) error {
	return nil
}

func (f *fakeIntegrationStore) ListByWorkspace(ctx context.Context, workspaceID int64) ([]model.Integration, error) {
	return nil, nil
}

func (f *fakeIntegrationStore) ListByOrganization(ctx context.Context, orgID int64) ([]model.Integration, error) {
	return nil, nil
}

func (f *fakeIntegrationStore) ListByCapability(ctx context.Context, workspaceID int64, capability model.Capability) ([]model.Integration, error) {
	return nil, nil
}

type fakeCredentialStore struct {
	existing []model.IntegrationCredential
	created  []model.IntegrationCredential
	mu       sync.Mutex
}

func newFakeCredentialStore() *fakeCredentialStore {
	return &fakeCredentialStore{}
}

func (f *fakeCredentialStore) GetByID(ctx context.Context, id int64) (*model.IntegrationCredential, error) {
	for _, c := range f.existing {
		if c.ID == id {
			return &c, nil
		}
	}
	return nil, store.ErrNotFound
}

func (f *fakeCredentialStore) GetPrimaryByIntegration(ctx context.Context, integrationID int64) (*model.IntegrationCredential, error) {
	for _, c := range f.existing {
		if c.IntegrationID == integrationID && c.IsPrimary {
			return &c, nil
		}
	}
	return nil, store.ErrNotFound
}

func (f *fakeCredentialStore) GetByIntegrationAndUser(ctx context.Context, integrationID int64, userID int64) (*model.IntegrationCredential, error) {
	return nil, store.ErrNotFound
}

func (f *fakeCredentialStore) Create(ctx context.Context, cred *model.IntegrationCredential) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	copy := *cred
	f.created = append(f.created, copy)
	return nil
}

func (f *fakeCredentialStore) UpdateTokens(ctx context.Context, id int64, accessToken string, refreshToken *string, expiresAt *time.Time) error {
	return nil
}

func (f *fakeCredentialStore) SetAsPrimary(ctx context.Context, integrationID int64, credentialID int64) error {
	return nil
}

func (f *fakeCredentialStore) Revoke(ctx context.Context, id int64) error {
	return nil
}

func (f *fakeCredentialStore) RevokeAllByIntegration(ctx context.Context, integrationID int64) error {
	return nil
}

func (f *fakeCredentialStore) Delete(ctx context.Context, id int64) error {
	return nil
}

func (f *fakeCredentialStore) ListByIntegration(ctx context.Context, integrationID int64) ([]model.IntegrationCredential, error) {
	return nil, nil
}

func (f *fakeCredentialStore) ListActiveByIntegration(ctx context.Context, integrationID int64) ([]model.IntegrationCredential, error) {
	return nil, nil
}

func findCredentialType(creds []model.IntegrationCredential, t model.CredentialType) bool {
	for _, c := range creds {
		if c.CredentialType == t {
			return true
		}
	}
	return false
}

type fakeStoreProvider struct {
	integrationStore       store.IntegrationStore
	credentialStore        store.IntegrationCredentialStore
	integrationConfigStore store.IntegrationConfigStore
	repoStore              store.RepoStore
}

func (f *fakeStoreProvider) Integrations() store.IntegrationStore {
	return f.integrationStore
}

func (f *fakeStoreProvider) IntegrationCredentials() store.IntegrationCredentialStore {
	return f.credentialStore
}

func (f *fakeStoreProvider) IntegrationConfigs() store.IntegrationConfigStore {
	return f.integrationConfigStore
}

func (f *fakeStoreProvider) Repos() store.RepoStore {
	return f.repoStore
}

type fakeTxRunner struct {
	provider StoreProvider
}

func (f *fakeTxRunner) WithTx(ctx context.Context, fn func(stores StoreProvider) error) error {
	return fn(f.provider)
}

type fakeRepoStore struct {
	existing []model.Repository
	created  []model.Repository
	mu       sync.Mutex
}

func newFakeRepoStore() *fakeRepoStore {
	return &fakeRepoStore{}
}

func (f *fakeRepoStore) GetByID(ctx context.Context, id int64) (*model.Repository, error) {
	return nil, store.ErrNotFound
}

func (f *fakeRepoStore) GetByExternalID(ctx context.Context, integrationID int64, externalRepoID string) (*model.Repository, error) {
	return nil, store.ErrNotFound
}

func (f *fakeRepoStore) Create(ctx context.Context, repo *model.Repository) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	copy := *repo
	f.created = append(f.created, copy)
	return nil
}

func (f *fakeRepoStore) Update(ctx context.Context, repo *model.Repository) error {
	return nil
}

func (f *fakeRepoStore) Delete(ctx context.Context, id int64) error {
	return nil
}

func (f *fakeRepoStore) DeleteByIntegration(ctx context.Context, integrationID int64) error {
	return nil
}

func (f *fakeRepoStore) ListByWorkspace(ctx context.Context, workspaceID int64) ([]model.Repository, error) {
	return nil, nil
}

func (f *fakeRepoStore) ListByIntegration(ctx context.Context, integrationID int64) ([]model.Repository, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]model.Repository, 0, len(f.existing)+len(f.created))
	out = append(out, f.existing...)
	out = append(out, f.created...)
	return out, nil
}

type fakeIntegrationConfigStore struct {
	existing []model.IntegrationConfig
	created  []model.IntegrationConfig
	updated  []model.IntegrationConfig
	mu       sync.Mutex
}

func newFakeIntegrationConfigStore() *fakeIntegrationConfigStore {
	return &fakeIntegrationConfigStore{}
}

func (f *fakeIntegrationConfigStore) GetByID(ctx context.Context, id int64) (*model.IntegrationConfig, error) {
	for _, c := range f.existing {
		if c.ID == id {
			return &c, nil
		}
	}
	return nil, store.ErrNotFound
}

func (f *fakeIntegrationConfigStore) GetByIntegrationAndKey(ctx context.Context, integrationID int64, key string) (*model.IntegrationConfig, error) {
	for _, c := range f.existing {
		if c.IntegrationID == integrationID && c.Key == key {
			return &c, nil
		}
	}
	return nil, store.ErrNotFound
}

func (f *fakeIntegrationConfigStore) ListByIntegration(ctx context.Context, integrationID int64) ([]model.IntegrationConfig, error) {
	var result []model.IntegrationConfig
	for _, c := range f.existing {
		if c.IntegrationID == integrationID {
			result = append(result, c)
		}
	}
	return result, nil
}

func (f *fakeIntegrationConfigStore) ListByIntegrationAndType(ctx context.Context, integrationID int64, configType string) ([]model.IntegrationConfig, error) {
	var result []model.IntegrationConfig
	for _, c := range f.existing {
		if c.IntegrationID == integrationID && c.ConfigType == configType {
			result = append(result, c)
		}
	}
	return result, nil
}

func (f *fakeIntegrationConfigStore) Create(ctx context.Context, config *model.IntegrationConfig) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	copy := *config
	f.created = append(f.created, copy)
	return nil
}

func (f *fakeIntegrationConfigStore) Update(ctx context.Context, config *model.IntegrationConfig) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	copy := *config
	f.updated = append(f.updated, copy)
	return nil
}

func (f *fakeIntegrationConfigStore) Upsert(ctx context.Context, config *model.IntegrationConfig) error {
	return f.Create(ctx, config)
}

func (f *fakeIntegrationConfigStore) Delete(ctx context.Context, id int64) error {
	return nil
}

func (f *fakeIntegrationConfigStore) DeleteByIntegration(ctx context.Context, integrationID int64) error {
	return nil
}
