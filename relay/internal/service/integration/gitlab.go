package integration

import (
	"context"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type GitLabService interface {
	TestConnection(ctx context.Context, instanceURL, token string) (*TestConnectionResult, error)
}

type TestConnectionResult struct {
	Username     string
	ProjectCount int
}

type gitLabService struct{}

func NewGitLabService() GitLabService {
	return &gitLabService{}
}

func (s *gitLabService) TestConnection(ctx context.Context, instanceURL, token string) (*TestConnectionResult, error) {
	baseURL := strings.TrimSuffix(instanceURL, "/") + "/api/v4"

	client, err := gitlab.NewClient(
		token,
		gitlab.WithBaseURL(baseURL),
	)
	if err != nil {
		return nil, err
	}

	user, _, err := client.Users.CurrentUser(gitlab.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	projects, resp, err := client.Projects.ListProjects(
		&gitlab.ListProjectsOptions{
			MinAccessLevel: gitlab.Ptr(gitlab.DeveloperPermissions),
			ListOptions: gitlab.ListOptions{
				Page:    1,
				PerPage: 1,
			},
		},
		gitlab.WithContext(ctx),
	)
	if err != nil {
		return nil, err
	}

	count := int(resp.TotalItems)
	if count == 0 {
		count = len(projects)
	}

	return &TestConnectionResult{
		Username:     user.Username,
		ProjectCount: count,
	}, nil
}
