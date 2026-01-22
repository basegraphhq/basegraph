package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"basegraph.co/relay/common/id"
	"basegraph.co/relay/core/config"
	"basegraph.co/relay/internal/model"
	"basegraph.co/relay/internal/store"
	"github.com/workos/workos-go/v6/pkg/usermanagement"
)

var (
	ErrInvalidCode    = errors.New("invalid authorization code")
	ErrUserNotFound   = errors.New("user not found")
	ErrSessionExpired = errors.New("session expired")
)

type AuthService interface {
	GetAuthorizationURL(state string, opts ...AuthURLOption) (string, error)
	HandleCallback(ctx context.Context, code string) (*CallbackResult, error)
	HandleSignIn(ctx context.Context, code string) (*CallbackResult, error)
	ValidateSession(ctx context.Context, sessionID int64) (*model.User, *UserContext, error)
	GetSessionByID(ctx context.Context, sessionID int64) (*model.Session, error)
	Logout(ctx context.Context, sessionID int64) error
	GetLogoutURL(workosSessionID string, returnTo string) string
}

// CallbackResult contains the result of a successful OAuth callback
type CallbackResult struct {
	User            *model.User
	Session         *model.Session
	WorkOSSessionID string // The WorkOS session ID (sid claim from access token)
}

// AuthURLOption configures authorization URL generation
type AuthURLOption func(*authURLOptions)

type authURLOptions struct {
	loginHint string
}

// WithLoginHint pre-fills the email field in the auth flow
func WithLoginHint(email string) AuthURLOption {
	return func(o *authURLOptions) {
		o.loginHint = email
	}
}

type UserContext struct {
	OrganizationID  *int64
	WorkspaceID     *int64
	HasOrganization bool
}

type authService struct {
	userStore      store.UserStore
	sessionStore   store.SessionStore
	orgStore       store.OrganizationStore
	workspaceStore store.WorkspaceStore
	cfg            config.WorkOSConfig
	dashboardURL   string
}

func NewAuthService(
	userStore store.UserStore,
	sessionStore store.SessionStore,
	orgStore store.OrganizationStore,
	workspaceStore store.WorkspaceStore,
	cfg config.WorkOSConfig,
	dashboardURL string,
) AuthService {
	usermanagement.SetAPIKey(cfg.APIKey)
	return &authService{
		userStore:      userStore,
		sessionStore:   sessionStore,
		orgStore:       orgStore,
		workspaceStore: workspaceStore,
		cfg:            cfg,
		dashboardURL:   dashboardURL,
	}
}

func (s *authService) GetAuthorizationURL(state string, opts ...AuthURLOption) (string, error) {
	options := &authURLOptions{}
	for _, opt := range opts {
		opt(options)
	}

	urlOpts := usermanagement.GetAuthorizationURLOpts{
		ClientID:    s.cfg.ClientID,
		RedirectURI: s.cfg.RedirectURI,
		State:       state,
		Provider:    "authkit",
	}

	// Set login hint if provided (pre-fills email in auth flow)
	if options.loginHint != "" {
		urlOpts.LoginHint = options.loginHint
	}

	url, err := usermanagement.GetAuthorizationURL(urlOpts)
	if err != nil {
		return "", fmt.Errorf("generating authorization URL: %w", err)
	}
	return url.String(), nil
}

func (s *authService) HandleCallback(ctx context.Context, code string) (*CallbackResult, error) {
	authResponse, err := usermanagement.AuthenticateWithCode(ctx, usermanagement.AuthenticateWithCodeOpts{
		ClientID: s.cfg.ClientID,
		Code:     code,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to authenticate with code", "error", err)
		return nil, ErrInvalidCode
	}

	workosUser := authResponse.User

	var avatarURL *string
	if workosUser.ProfilePictureURL != "" {
		avatarURL = &workosUser.ProfilePictureURL
	}

	user := &model.User{
		ID:        id.New(),
		Name:      buildUserName(workosUser),
		Email:     workosUser.Email,
		AvatarURL: avatarURL,
		WorkOSID:  &workosUser.ID,
	}

	if err := s.userStore.Upsert(ctx, user); err != nil {
		slog.ErrorContext(ctx, "failed to upsert user",
			"error", err,
			"email", user.Email,
			"workos_id", workosUser.ID,
		)
		return nil, fmt.Errorf("upserting user: %w", err)
	}

	// Extract WorkOS session ID from access token for logout URL building
	workosSessionID := extractSessionIDFromToken(authResponse.AccessToken)
	var workosSessionIDPtr *string
	if workosSessionID != "" {
		workosSessionIDPtr = &workosSessionID
	}

	session := &model.Session{
		ID:              id.New(),
		UserID:          user.ID,
		ExpiresAt:       time.Now().Add(7 * 24 * time.Hour),
		WorkOSSessionID: workosSessionIDPtr,
	}

	if err := s.sessionStore.Create(ctx, session); err != nil {
		slog.ErrorContext(ctx, "failed to create session",
			"error", err,
			"user_id", user.ID,
		)
		return nil, fmt.Errorf("creating session: %w", err)
	}

	slog.InfoContext(ctx, "user authenticated",
		"user_id", user.ID,
		"email", user.Email,
		"session_id", session.ID,
	)

	return &CallbackResult{
		User:            user,
		Session:         session,
		WorkOSSessionID: workosSessionID,
	}, nil
}

func (s *authService) HandleSignIn(ctx context.Context, code string) (*CallbackResult, error) {
	authResponse, err := usermanagement.AuthenticateWithCode(ctx, usermanagement.AuthenticateWithCodeOpts{
		ClientID: s.cfg.ClientID,
		Code:     code,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to authenticate with code", "error", err)
		return nil, ErrInvalidCode
	}

	workosUser := authResponse.User

	// Lookup user by email - DO NOT create new users
	user, err := s.userStore.GetByEmail(ctx, workosUser.Email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			slog.WarnContext(ctx, "sign-in attempted by non-existent user",
				"email", workosUser.Email,
			)
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("looking up user: %w", err)
	}

	// Update user info from WorkOS (name, avatar, workos_id) without creating
	var avatarURL *string
	if workosUser.ProfilePictureURL != "" {
		avatarURL = &workosUser.ProfilePictureURL
	}
	user.Name = buildUserName(workosUser)
	user.AvatarURL = avatarURL
	user.WorkOSID = &workosUser.ID

	if err := s.userStore.Update(ctx, user); err != nil {
		slog.WarnContext(ctx, "failed to update user info on sign-in",
			"error", err,
			"user_id", user.ID,
		)
		// Non-fatal - continue with sign-in
	}

	// Extract WorkOS session ID from access token for logout URL building
	workosSessionID := extractSessionIDFromToken(authResponse.AccessToken)
	var workosSessionIDPtr *string
	if workosSessionID != "" {
		workosSessionIDPtr = &workosSessionID
	}

	session := &model.Session{
		ID:              id.New(),
		UserID:          user.ID,
		ExpiresAt:       time.Now().Add(7 * 24 * time.Hour),
		WorkOSSessionID: workosSessionIDPtr,
	}

	if err := s.sessionStore.Create(ctx, session); err != nil {
		slog.ErrorContext(ctx, "failed to create session",
			"error", err,
			"user_id", user.ID,
		)
		return nil, fmt.Errorf("creating session: %w", err)
	}

	slog.InfoContext(ctx, "user signed in",
		"user_id", user.ID,
		"email", user.Email,
		"session_id", session.ID,
	)

	return &CallbackResult{
		User:            user,
		Session:         session,
		WorkOSSessionID: workosSessionID,
	}, nil
}

func (s *authService) ValidateSession(ctx context.Context, sessionID int64) (*model.User, *UserContext, error) {
	session, err := s.sessionStore.GetValid(ctx, sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, ErrSessionExpired
		}
		return nil, nil, fmt.Errorf("getting session: %w", err)
	}

	user, err := s.userStore.GetByID(ctx, session.UserID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, ErrUserNotFound
		}
		return nil, nil, fmt.Errorf("getting user: %w", err)
	}

	userCtx := &UserContext{}

	orgs, err := s.orgStore.ListByAdminUser(ctx, user.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("checking organization membership: %w", err)
	}

	if len(orgs) > 0 {
		userCtx.HasOrganization = true
		userCtx.OrganizationID = &orgs[0].ID

		workspaces, err := s.workspaceStore.ListByUser(ctx, user.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("listing workspaces: %w", err)
		}
		if len(workspaces) > 0 {
			userCtx.WorkspaceID = &workspaces[0].ID
		}
	}

	return user, userCtx, nil
}

func (s *authService) GetSessionByID(ctx context.Context, sessionID int64) (*model.Session, error) {
	session, err := s.sessionStore.GetByID(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("getting session: %w", err)
	}
	return session, nil
}

func (s *authService) Logout(ctx context.Context, sessionID int64) error {
	if err := s.sessionStore.Delete(ctx, sessionID); err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}
	return nil
}

// GetLogoutURL builds a WorkOS logout URL that will end the user's WorkOS session
// and redirect them to the specified returnTo URL.
func (s *authService) GetLogoutURL(workosSessionID string, returnTo string) string {
	logoutURL, err := usermanagement.GetLogoutURL(usermanagement.GetLogoutURLOpts{
		SessionID: workosSessionID,
		ReturnTo:  returnTo,
	})
	if err != nil {
		// If we can't build the URL (shouldn't happen), return empty string
		return ""
	}
	return logoutURL.String()
}

func buildUserName(user usermanagement.User) string {
	if user.FirstName != "" && user.LastName != "" {
		return user.FirstName + " " + user.LastName
	}
	if user.FirstName != "" {
		return user.FirstName
	}
	if user.LastName != "" {
		return user.LastName
	}
	return user.Email
}

// extractSessionIDFromToken extracts the 'sid' (session ID) claim from a WorkOS access token.
// The token is a JWT and we extract the claim without signature verification since
// we just received it from WorkOS. Returns empty string if extraction fails.
func extractSessionIDFromToken(accessToken string) string {
	if accessToken == "" {
		return ""
	}

	// JWT format: header.payload.signature
	parts := strings.Split(accessToken, ".")
	if len(parts) != 3 {
		return ""
	}

	// Decode the payload (middle part)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}

	// Parse as JSON to extract 'sid' claim
	var claims struct {
		SessionID string `json:"sid"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}

	return claims.SessionID
}
