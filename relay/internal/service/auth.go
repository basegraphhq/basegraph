package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"basegraph.app/relay/common/id"
	"basegraph.app/relay/core/config"
	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/store"
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
	ValidateSession(ctx context.Context, sessionID int64) (*model.User, error)
	Logout(ctx context.Context, sessionID int64) error
}

type authService struct {
	userStore    store.UserStore
	sessionStore store.SessionStore
	orgStore     store.OrganizationStore
	cfg          config.WorkOSConfig
	dashboardURL string
}

func NewAuthService(
	userStore store.UserStore,
	sessionStore store.SessionStore,
	orgStore store.OrganizationStore,
	cfg config.WorkOSConfig,
	dashboardURL string,
) AuthService {
	usermanagement.SetAPIKey(cfg.APIKey)
	return &authService{
		userStore:    userStore,
		sessionStore: sessionStore,
		orgStore:     orgStore,
		cfg:          cfg,
		dashboardURL: dashboardURL,
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

func (s *authService) ValidateSession(ctx context.Context, sessionID int64) (*model.User, error) {
	session, err := s.sessionStore.GetValid(ctx, sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrSessionExpired
		}
		return nil, fmt.Errorf("getting session: %w", err)
	}

	user, err := s.userStore.GetByID(ctx, session.UserID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("getting user: %w", err)
	}

	return user, nil
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
