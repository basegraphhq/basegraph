package handler

import (
	"net/http"

	"basegraph.app/relay/internal/http/dto"
	"basegraph.app/relay/internal/service/integration"
	"github.com/gin-gonic/gin"
)

type GitLabHandler struct {
	gitlabService integration.GitLabService
}

func NewGitLabHandler(gitlabService integration.GitLabService) *GitLabHandler {
	return &GitLabHandler{gitlabService: gitlabService}
}

func (h *GitLabHandler) TestConnection(c *gin.Context) {
	ctx := c.Request.Context()

	var req dto.TestGitLabConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.gitlabService.TestConnection(ctx, req.InstanceURL, req.Token)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to connect to gitlab"})
		return
	}

	c.JSON(http.StatusOK, dto.TestGitLabConnectionResponse{
		Username:     result.Username,
		ProjectCount: result.ProjectCount,
	})
}
