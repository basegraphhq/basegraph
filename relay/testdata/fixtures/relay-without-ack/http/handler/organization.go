package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"basegraph.app/relay/internal/http/dto"
	"basegraph.app/relay/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
)

type OrganizationHandler struct {
	orgService service.OrganizationService
}

func NewOrganizationHandler(orgService service.OrganizationService) *OrganizationHandler {
	return &OrganizationHandler{orgService: orgService}
}

func (h *OrganizationHandler) Create(c *gin.Context) {
	ctx := c.Request.Context()

	var req dto.CreateOrganizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.WarnContext(ctx, "invalid organization request body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	org, err := h.orgService.Create(ctx, req.Name, req.Slug, req.AdminUserID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			slog.InfoContext(ctx, "duplicate organization slug", "slug", req.Slug)
			c.JSON(http.StatusConflict, gin.H{"error": "organization with this slug already exists"})
			return
		}

		slog.ErrorContext(ctx, "failed to create organization", "error", err, "admin_user_id", req.AdminUserID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create organization"})
		return
	}

	c.JSON(http.StatusCreated, dto.ToOrganizationResponse(org))
}
