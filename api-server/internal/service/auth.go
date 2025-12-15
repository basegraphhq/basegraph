package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"basegraph.app/api-server/common/id"
	"basegraph.app/api-server/core/config"
	"basegraph.app/api-server/internal/model"
	"basegraph.app/api-server/internal/store"
	"github.com/workos/workos-go/v6/pkg/usermanagement"
)

var (
	ErrInvalidCode    = errors.New("invalid authorization code")
	ErrUserNotFound   = errors.New("user not found")
	ErrSessionExpired = errors.New("session expired")
)

type AuthService interface {
	GetAuthorizationURL(state string) (string, error)
	HandleCallback(ctx context.Context, code string) (*model.User, *model.Session, error)
	ValidateSession(ctx context.Context, sessionID int64) (*model.User, *UserContext, error)
	Logout(ctx context.Context, sessionID int64) error
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

func (s *authService) GetAuthorizationURL(state string) (string, error) {
	url, err := usermanagement.GetAuthorizationURL(usermanagement.GetAuthorizationURLOpts{
		ClientID:    s.cfg.ClientID,
		RedirectURI: s.cfg.RedirectURI,
		State:       state,
		Provider:    "authkit",
	})
	if err != nil {
		return "", fmt.Errorf("generating authorization URL: %w", err)
	}
	return url.String(), nil
}

func (s *authService) HandleCallback(ctx context.Context, code string) (*model.User, *model.Session, error) {
	authResponse, err := usermanagement.AuthenticateWithCode(ctx, usermanagement.AuthenticateWithCodeOpts{
		ClientID: s.cfg.ClientID,
		Code:     code,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to authenticate with code", "error", err)
		return nil, nil, ErrInvalidCode
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

	if err := s.userStore.UpsertByWorkOSID(ctx, user); err != nil {
		slog.ErrorContext(ctx, "failed to upsert user",
			"error", err,
			"email", user.Email,
			"workos_id", workosUser.ID,
		)
		return nil, nil, fmt.Errorf("upserting user: %w", err)
	}

	session := &model.Session{
		ID:        id.New(),
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}

	if err := s.sessionStore.Create(ctx, session); err != nil {
		slog.ErrorContext(ctx, "failed to create session",
			"error", err,
			"user_id", user.ID,
		)
		return nil, nil, fmt.Errorf("creating session: %w", err)
	}

	slog.InfoContext(ctx, "user authenticated",
		"user_id", user.ID,
		"email", user.Email,
		"session_id", session.ID,
	)

	return user, session, nil
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

func (s *authService) Logout(ctx context.Context, sessionID int64) error {
	if err := s.sessionStore.Delete(ctx, sessionID); err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}
	return nil
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
