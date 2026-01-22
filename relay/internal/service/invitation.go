package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"basegraph.co/relay/common/id"
	"basegraph.co/relay/internal/model"
	"basegraph.co/relay/internal/store"
)

const (
	InviteTokenLength = 32
	InviteExpiryDays  = 7
)

var (
	ErrInviteNotFound      = errors.New("invitation not found")
	ErrInviteExpired       = errors.New("invitation has expired")
	ErrInviteAlreadyUsed   = errors.New("invitation has already been used")
	ErrInviteRevoked       = errors.New("invitation has been revoked")
	ErrEmailMismatch       = errors.New("authenticated email does not match invitation")
	ErrInvitePendingExists = errors.New("a pending invitation already exists for this email")
)

type InvitationService interface {
	Create(ctx context.Context, email string, invitedBy *int64) (*model.Invitation, string, error)
	ValidateToken(ctx context.Context, token string) (*model.Invitation, error)
	GetByToken(ctx context.Context, token string) (*model.Invitation, error)
	Accept(ctx context.Context, token string, user *model.User) (*model.Invitation, error)
	Revoke(ctx context.Context, id int64) (*model.Invitation, error)
	List(ctx context.Context, limit, offset int32) ([]model.Invitation, error)
	ListPending(ctx context.Context) ([]model.Invitation, error)
}

type invitationService struct {
	invStore     store.InvitationStore
	dashboardURL string
}

func NewInvitationService(invStore store.InvitationStore, dashboardURL string) InvitationService {
	return &invitationService{
		invStore:     invStore,
		dashboardURL: dashboardURL,
	}
}

func (s *invitationService) Create(ctx context.Context, email string, invitedBy *int64) (*model.Invitation, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	// Check if there's already a pending invitation for this email
	existing, err := s.invStore.GetByEmail(ctx, email)
	if err == nil && existing != nil && existing.IsValid() {
		return nil, "", ErrInvitePendingExists
	}

	token, err := generateSecureToken(InviteTokenLength)
	if err != nil {
		return nil, "", fmt.Errorf("generating token: %w", err)
	}

	inv := &model.Invitation{
		ID:        id.New(),
		Email:     email,
		Token:     token,
		Status:    model.InvitationStatusPending,
		InvitedBy: invitedBy,
		ExpiresAt: time.Now().Add(InviteExpiryDays * 24 * time.Hour),
	}

	if err := s.invStore.Create(ctx, inv); err != nil {
		return nil, "", fmt.Errorf("creating invitation: %w", err)
	}

	inviteURL := fmt.Sprintf("%s/invite?token=%s", s.dashboardURL, token)

	slog.InfoContext(ctx, "invitation created",
		"invitation_id", inv.ID,
		"email", email,
		"expires_at", inv.ExpiresAt,
	)

	return inv, inviteURL, nil
}

func (s *invitationService) ValidateToken(ctx context.Context, token string) (*model.Invitation, error) {
	inv, err := s.invStore.GetValidByToken(ctx, token)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Try to get by token to determine if expired/used/revoked
			inv, err := s.invStore.GetByToken(ctx, token)
			if err != nil {
				return nil, ErrInviteNotFound
			}
			switch inv.Status {
			case model.InvitationStatusAccepted:
				return nil, ErrInviteAlreadyUsed
			case model.InvitationStatusRevoked:
				return nil, ErrInviteRevoked
			case model.InvitationStatusExpired:
				return nil, ErrInviteExpired
			default:
				if time.Now().After(inv.ExpiresAt) {
					return nil, ErrInviteExpired
				}
				return nil, ErrInviteNotFound
			}
		}
		return nil, fmt.Errorf("getting invitation: %w", err)
	}

	return inv, nil
}

func (s *invitationService) GetByToken(ctx context.Context, token string) (*model.Invitation, error) {
	inv, err := s.invStore.GetByToken(ctx, token)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrInviteNotFound
		}
		return nil, fmt.Errorf("getting invitation: %w", err)
	}
	return inv, nil
}

func (s *invitationService) Accept(ctx context.Context, token string, user *model.User) (*model.Invitation, error) {
	inv, err := s.ValidateToken(ctx, token)
	if err != nil {
		return nil, err
	}

	// Check email matches
	if !strings.EqualFold(inv.Email, user.Email) {
		slog.WarnContext(ctx, "email mismatch on invitation acceptance",
			"invitation_email", inv.Email,
			"user_email", user.Email,
			"invitation_id", inv.ID,
		)
		return nil, ErrEmailMismatch
	}

	accepted, err := s.invStore.Accept(ctx, inv.ID, user.ID)
	if err != nil {
		return nil, fmt.Errorf("accepting invitation: %w", err)
	}

	slog.InfoContext(ctx, "invitation accepted",
		"invitation_id", inv.ID,
		"user_id", user.ID,
		"email", user.Email,
	)

	return accepted, nil
}

func (s *invitationService) Revoke(ctx context.Context, id int64) (*model.Invitation, error) {
	inv, err := s.invStore.Revoke(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrInviteNotFound
		}
		return nil, fmt.Errorf("revoking invitation: %w", err)
	}

	slog.InfoContext(ctx, "invitation revoked",
		"invitation_id", id,
		"email", inv.Email,
	)

	return inv, nil
}

func (s *invitationService) List(ctx context.Context, limit, offset int32) ([]model.Invitation, error) {
	return s.invStore.List(ctx, limit, offset)
}

func (s *invitationService) ListPending(ctx context.Context) ([]model.Invitation, error) {
	return s.invStore.ListPending(ctx)
}

func generateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}
