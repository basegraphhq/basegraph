package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"basegraph.co/relay/internal/http/dto"
	"basegraph.co/relay/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
)

type UserHandler struct {
	userService service.UserService
}

func NewUserHandler(userService service.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

func (h *UserHandler) Create(c *gin.Context) {
	ctx := c.Request.Context()

	var req dto.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.WarnContext(ctx, "invalid request body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.userService.Create(ctx, req.Name, req.Email, req.AvatarURL)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			slog.InfoContext(ctx, "duplicate user creation attempted", "email", req.Email)
			c.JSON(http.StatusConflict, gin.H{"error": "user with this email already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	c.JSON(http.StatusCreated, dto.ToUserResponse(user))
}

func (h *UserHandler) GetByID(c *gin.Context) {
}

func (h *UserHandler) Sync(c *gin.Context) {
	ctx := c.Request.Context()

	var req dto.SyncUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.WarnContext(ctx, "invalid sync request body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, orgs, err := h.userService.Sync(ctx, req.Name, req.Email, req.AvatarURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sync user"})
		return
	}

	resp := dto.SyncUserResponse{
		User:            dto.ToUserResponse(user),
		Organizations:   make([]dto.OrganizationBrief, 0, len(orgs)),
		HasOrganization: len(orgs) > 0,
	}
	for _, org := range orgs {
		resp.Organizations = append(resp.Organizations, dto.ToOrganizationBrief(org))
	}

	c.JSON(http.StatusOK, resp)
}
