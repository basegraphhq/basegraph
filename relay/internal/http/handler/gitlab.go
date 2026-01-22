package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"basegraph.co/relay/internal/http/dto"
	"basegraph.co/relay/internal/service/integration"
	"github.com/gin-gonic/gin"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type GitLabHandler struct {
	gitlabService  integration.GitLabService
	webhookBaseURL string
}

func NewGitLabHandler(gitlabService integration.GitLabService, webhookBaseURL string) *GitLabHandler {
	return &GitLabHandler{
		gitlabService:  gitlabService,
		webhookBaseURL: webhookBaseURL,
	}
}

func (h *GitLabHandler) ListProjects(c *gin.Context) {
	ctx := c.Request.Context()

	var req dto.ListGitLabProjectsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	projects, err := h.gitlabService.ListProjects(ctx, req.InstanceURL, req.Token)
	if err != nil {
		slog.ErrorContext(ctx, "failed to list gitlab projects", "error", err)
		statusCode, errorMsg := mapGitLabError(err)
		c.JSON(statusCode, gin.H{"error": errorMsg})
		return
	}

	resp := make([]dto.GitLabProjectResponse, 0, len(projects))
	for _, p := range projects {
		resp = append(resp, dto.GitLabProjectResponse{
			ID:          p.ID,
			Name:        p.Name,
			PathWithNS:  p.PathWithNS,
			WebURL:      p.WebURL,
			Description: p.Description,
		})
	}

	c.JSON(http.StatusOK, resp)
}

func (h *GitLabHandler) SetupIntegration(c *gin.Context) {
	ctx := c.Request.Context()

	var req dto.SetupGitLabIntegrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.gitlabService.SetupIntegration(ctx, integration.SetupIntegrationParams{
		InstanceURL:    req.InstanceURL,
		Token:          req.Token,
		WorkspaceID:    req.WorkspaceID,
		OrganizationID: req.OrganizationID,
		SetupByUserID:  req.SetupByUserID,
		WebhookBaseURL: h.webhookBaseURL,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to setup gitlab integration", "error", err)
		statusCode, errorMsg := mapGitLabError(err)
		c.JSON(statusCode, gin.H{"error": errorMsg})
		return
	}

	projects := make([]dto.GitLabProjectResponse, 0, len(result.Projects))
	for _, p := range result.Projects {
		projects = append(projects, dto.GitLabProjectResponse{
			ID:          p.ID,
			Name:        p.Name,
			PathWithNS:  p.PathWithNS,
			WebURL:      p.WebURL,
			Description: p.Description,
		})
	}

	c.JSON(http.StatusOK, dto.SetupGitLabIntegrationResponse{
		IntegrationID:     result.IntegrationID,
		IsNewIntegration:  result.IsNewIntegration,
		Projects:          projects,
		RepositoriesAdded: result.RepositoriesAdded,
		WebhooksCreated:   result.WebhooksCreated,
		Errors:            result.Errors,
	})
}

type gitLabWorkspaceQuery struct {
	WorkspaceID int64 `form:"workspace_id" binding:"required"`
}

func (h *GitLabHandler) GetStatus(c *gin.Context) {
	ctx := c.Request.Context()

	var req gitLabWorkspaceQuery
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	status, err := h.gitlabService.Status(ctx, req.WorkspaceID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to get gitlab status", "error", err)
		statusCode, errorMsg := mapGitLabError(err)
		c.JSON(statusCode, gin.H{"error": errorMsg})
		return
	}

	var statusPayload *dto.GitLabSyncStatus
	if status.Connected {
		statusPayload = &dto.GitLabSyncStatus{
			Synced:            status.Synced,
			WebhooksCreated:   status.WebhooksCreated,
			RepositoriesAdded: status.RepositoriesAdded,
			Errors:            status.Errors,
		}
		if status.UpdatedAt != nil {
			statusPayload.UpdatedAt = status.UpdatedAt.UTC().Format(time.RFC3339)
		}
	}

	c.JSON(http.StatusOK, dto.GitLabStatusResponse{
		Connected:     status.Connected,
		IntegrationID: status.IntegrationID,
		Status:        statusPayload,
		ReposCount:    status.ReposCount,
	})
}

type gitLabWorkspaceBody struct {
	WorkspaceID int64 `json:"workspace_id,string" binding:"required"`
}

func (h *GitLabHandler) RefreshIntegration(c *gin.Context) {
	ctx := c.Request.Context()

	var req gitLabWorkspaceBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.gitlabService.RefreshIntegration(ctx, req.WorkspaceID, h.webhookBaseURL)
	if err != nil {
		slog.ErrorContext(ctx, "failed to refresh gitlab integration", "error", err)
		statusCode, errorMsg := mapGitLabError(err)
		c.JSON(statusCode, gin.H{"error": errorMsg})
		return
	}

	synced := result.WebhooksCreated > 0 || result.RepositoriesAdded > 0

	projects := make([]dto.GitLabProjectResponse, 0, len(result.Projects))
	for _, p := range result.Projects {
		projects = append(projects, dto.GitLabProjectResponse{
			ID:          p.ID,
			Name:        p.Name,
			PathWithNS:  p.PathWithNS,
			WebURL:      p.WebURL,
			Description: p.Description,
		})
	}

	c.JSON(http.StatusOK, dto.RefreshGitLabIntegrationResponse{
		SetupGitLabIntegrationResponse: dto.SetupGitLabIntegrationResponse{
			IntegrationID:     result.IntegrationID,
			IsNewIntegration:  result.IsNewIntegration,
			Projects:          projects,
			RepositoriesAdded: result.RepositoriesAdded,
			WebhooksCreated:   result.WebhooksCreated,
			Errors:            result.Errors,
		},
		Synced: synced,
	})
}

func mapGitLabError(err error) (int, string) {
	if err == nil {
		return http.StatusInternalServerError, "unknown error"
	}

	errorMsg := err.Error()

	var gitlabErr *gitlab.ErrorResponse
	if errors.As(err, &gitlabErr) {
		if gitlabErr.Response != nil {
			statusCode := gitlabErr.Response.StatusCode
			switch statusCode {
			case http.StatusUnauthorized:
				return http.StatusUnauthorized, "Invalid token. Please check your Personal Access Token and try again."
			case http.StatusForbidden:
				return http.StatusForbidden, "Token does not have sufficient permissions. Ensure the token has 'api' scope and the user has Maintainer access to at least one project."
			case http.StatusNotFound:
				return http.StatusBadRequest, "GitLab instance not found. Please check the instance URL."
			case http.StatusBadRequest:
				return http.StatusBadRequest, gitlabErr.Message
			default:
				return statusCode, gitlabErr.Message
			}
		}
		return http.StatusBadGateway, gitlabErr.Message
	}

	if strings.Contains(errorMsg, "no projects found with maintainer access") {
		return http.StatusBadRequest, errorMsg
	}

	if strings.Contains(errorMsg, "validating token") {
		if strings.Contains(errorMsg, "401") || strings.Contains(strings.ToLower(errorMsg), "unauthorized") {
			return http.StatusUnauthorized, "Invalid token. Please check your Personal Access Token and try again."
		}
		if strings.Contains(errorMsg, "403") || strings.Contains(strings.ToLower(errorMsg), "forbidden") {
			return http.StatusForbidden, "Token does not have sufficient permissions. Ensure the token has 'api' scope and the user has Maintainer access to at least one project."
		}
		return http.StatusBadRequest, errorMsg
	}

	return http.StatusBadGateway, errorMsg
}
