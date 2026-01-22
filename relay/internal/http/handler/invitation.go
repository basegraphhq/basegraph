package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/service"
	"github.com/gin-gonic/gin"
)

type InvitationHandler struct {
	invService  service.InvitationService
	adminAPIKey string
}

func NewInvitationHandler(invService service.InvitationService, adminAPIKey string) *InvitationHandler {
	return &InvitationHandler{
		invService:  invService,
		adminAPIKey: adminAPIKey,
	}
}

type createInvitationRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type createInvitationResponse struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	InviteURL string `json:"invite_url"`
	ExpiresAt string `json:"expires_at"`
}

// Create creates a new invitation (admin only)
func (h *InvitationHandler) Create(c *gin.Context) {
	ctx := c.Request.Context()

	var req createInvitationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: email is required"})
		return
	}

	inv, inviteURL, err := h.invService.Create(ctx, req.Email, nil)
	if err != nil {
		if errors.Is(err, service.ErrInvitePendingExists) {
			c.JSON(http.StatusConflict, gin.H{"error": "a pending invitation already exists for this email"})
			return
		}
		slog.ErrorContext(ctx, "failed to create invitation", "error", err, "email", req.Email)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create invitation"})
		return
	}

	slog.InfoContext(ctx, "invitation created via admin API",
		"invitation_id", inv.ID,
		"email", inv.Email,
	)

	c.JSON(http.StatusCreated, createInvitationResponse{
		ID:        inv.ID,
		Email:     inv.Email,
		InviteURL: inviteURL,
		ExpiresAt: inv.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

type listInvitationsResponse struct {
	Invitations []invitationResponse `json:"invitations"`
}

type invitationResponse struct {
	ID         int64   `json:"id"`
	Email      string  `json:"email"`
	Status     string  `json:"status"`
	ExpiresAt  string  `json:"expires_at"`
	CreatedAt  string  `json:"created_at"`
	AcceptedAt *string `json:"accepted_at,omitempty"`
}

// List lists all invitations (admin only)
func (h *InvitationHandler) List(c *gin.Context) {
	ctx := c.Request.Context()

	invitations, err := h.invService.List(ctx, 100, 0)
	if err != nil {
		slog.ErrorContext(ctx, "failed to list invitations", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list invitations"})
		return
	}

	resp := listInvitationsResponse{
		Invitations: make([]invitationResponse, len(invitations)),
	}
	for i, inv := range invitations {
		resp.Invitations[i] = toInvitationResponse(inv)
	}

	c.JSON(http.StatusOK, resp)
}

// ListPending lists pending invitations (admin only)
func (h *InvitationHandler) ListPending(c *gin.Context) {
	ctx := c.Request.Context()

	invitations, err := h.invService.ListPending(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to list pending invitations", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list invitations"})
		return
	}

	resp := listInvitationsResponse{
		Invitations: make([]invitationResponse, len(invitations)),
	}
	for i, inv := range invitations {
		resp.Invitations[i] = toInvitationResponse(inv)
	}

	c.JSON(http.StatusOK, resp)
}

type revokeInvitationRequest struct {
	ID int64 `json:"id" binding:"required"`
}

// Revoke revokes an invitation (admin only)
func (h *InvitationHandler) Revoke(c *gin.Context) {
	ctx := c.Request.Context()

	var req revokeInvitationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: id is required"})
		return
	}

	inv, err := h.invService.Revoke(ctx, req.ID)
	if err != nil {
		if errors.Is(err, service.ErrInviteNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "invitation not found or already processed"})
			return
		}
		slog.ErrorContext(ctx, "failed to revoke invitation", "error", err, "id", req.ID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke invitation"})
		return
	}

	slog.InfoContext(ctx, "invitation revoked via admin API",
		"invitation_id", inv.ID,
		"email", inv.Email,
	)

	c.JSON(http.StatusOK, toInvitationResponse(*inv))
}

type validateTokenResponse struct {
	Email     string `json:"email"`
	ExpiresAt string `json:"expires_at"`
	Valid     bool   `json:"valid"`
}

// Validate validates an invitation token (public endpoint)
func (h *InvitationHandler) Validate(c *gin.Context) {
	ctx := c.Request.Context()

	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token is required"})
		return
	}

	inv, err := h.invService.ValidateToken(ctx, token)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInviteNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "invitation not found", "code": "not_found"})
		case errors.Is(err, service.ErrInviteExpired):
			c.JSON(http.StatusGone, gin.H{"error": "invitation has expired", "code": "expired"})
		case errors.Is(err, service.ErrInviteAlreadyUsed):
			c.JSON(http.StatusGone, gin.H{"error": "invitation has already been used", "code": "already_used"})
		case errors.Is(err, service.ErrInviteRevoked):
			c.JSON(http.StatusGone, gin.H{"error": "invitation has been revoked", "code": "revoked"})
		default:
			slog.ErrorContext(ctx, "failed to validate invitation", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate invitation"})
		}
		return
	}

	c.JSON(http.StatusOK, validateTokenResponse{
		Email:     inv.Email,
		ExpiresAt: inv.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		Valid:     true,
	})
}

// RequireAdminAPIKey middleware checks for valid admin API key
func (h *InvitationHandler) RequireAdminAPIKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		if h.adminAPIKey == "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "admin API not configured"})
			c.Abort()
			return
		}

		apiKey := c.GetHeader("X-Admin-API-Key")
		if apiKey == "" {
			apiKey = c.GetHeader("Authorization")
			if len(apiKey) > 7 && apiKey[:7] == "Bearer " {
				apiKey = apiKey[7:]
			}
		}

		if apiKey != h.adminAPIKey {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or missing API key"})
			c.Abort()
			return
		}

		c.Next()
	}
}

func toInvitationResponse(inv model.Invitation) invitationResponse {
	resp := invitationResponse{
		ID:        inv.ID,
		Email:     inv.Email,
		Status:    string(inv.Status),
		ExpiresAt: inv.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		CreatedAt: inv.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if inv.AcceptedAt != nil {
		acceptedAt := inv.AcceptedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.AcceptedAt = &acceptedAt
	}
	return resp
}
