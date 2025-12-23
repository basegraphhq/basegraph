package issue_tracker

import (
	"context"
	"fmt"
	"strings"

	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/store"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type gitLabIssueTrackerService struct {
	integrations store.IntegrationStore
	credentials  store.IntegrationCredentialStore
}

func NewGitLabIssueTrackerService(
	integrations store.IntegrationStore,
	credentials store.IntegrationCredentialStore,
) IssueTrackerService {
	return &gitLabIssueTrackerService{
		integrations: integrations,
		credentials:  credentials,
	}
}

func (s *gitLabIssueTrackerService) FetchIssue(ctx context.Context, params FetchIssueParams) (*model.Issue, error) {
	client, err := s.getClient(ctx, params.IntegrationID)
	if err != nil {
		return nil, err
	}

	gitlabIssue, _, err := client.Issues.GetIssue(
		params.ProjectID,
		params.IssueIID,
		nil,
		gitlab.WithContext(ctx),
	)
	if err != nil {
		return nil, fmt.Errorf("fetching issue from gitlab: %w", err)
	}

	return s.mapToIssue(gitlabIssue), nil
}

func (s *gitLabIssueTrackerService) FetchDiscussions(ctx context.Context, params FetchDiscussionsParams) ([]model.Discussion, error) {
	client, err := s.getClient(ctx, params.IntegrationID)
	if err != nil {
		return nil, err
	}

	gitlabDiscussions, _, err := client.Discussions.ListIssueDiscussions(
		params.ProjectID,
		params.IssueIID,
		nil,
		gitlab.WithContext(ctx),
	)
	if err != nil {
		return nil, fmt.Errorf("fetching discussions from gitlab: %w", err)
	}

	return s.mapDiscussions(gitlabDiscussions), nil
}

func (s *gitLabIssueTrackerService) IsReplyToUser(ctx context.Context, params IsReplyParams) (bool, error) {
	if params.DiscussionID == "" {
		return false, nil
	}

	discussions, err := s.FetchDiscussions(ctx, FetchDiscussionsParams{
		IntegrationID: params.IntegrationID,
		ProjectID:     params.ProjectID,
		IssueIID:      params.IssueIID,
	})
	if err != nil {
		return false, fmt.Errorf("fetching discussions: %w", err)
	}

	// Check if any comment in the target thread was authored by the user
	expectedAuthor := fmt.Sprintf("id:%d", params.UserID)
	for _, d := range discussions {
		if d.ThreadID == nil || *d.ThreadID != params.DiscussionID {
			continue
		}
		if d.Author == expectedAuthor {
			return true, nil
		}
	}

	return false, nil
}

func (s *gitLabIssueTrackerService) getClient(ctx context.Context, integrationID int64) (*gitlab.Client, error) {
	integration, err := s.integrations.GetByID(ctx, integrationID)
	if err != nil {
		return nil, fmt.Errorf("fetching integration: %w", err)
	}

	cred, err := s.credentials.GetPrimaryByIntegration(ctx, integrationID)
	if err != nil {
		return nil, fmt.Errorf("fetching credential: %w", err)
	}

	client, err := s.newClient(integration.ProviderBaseURL, cred.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("creating gitlab client: %w", err)
	}

	return client, nil
}

func (s *gitLabIssueTrackerService) newClient(baseURL *string, token string) (*gitlab.Client, error) {
	if baseURL == nil || *baseURL == "" {
		return gitlab.NewClient(token)
	}
	apiURL := strings.TrimSuffix(*baseURL, "/") + "/api/v4"
	return gitlab.NewClient(token, gitlab.WithBaseURL(apiURL))
}

func (s *gitLabIssueTrackerService) mapDiscussions(gitlabDiscussions []*gitlab.Discussion) []model.Discussion {
	var discussions []model.Discussion

	for _, d := range gitlabDiscussions {
		if d == nil {
			continue
		}

		threadID := d.ID

		for _, n := range d.Notes {
			if n == nil {
				continue
			}

			author := fmt.Sprintf("id:%d", n.Author.ID)
			if n.Author.Username != "" {
				author = n.Author.Username
			}

			createdAt := n.CreatedAt
			if createdAt == nil {
				createdAt = n.UpdatedAt
			}

			discussion := model.Discussion{
				ExternalID: fmt.Sprintf("%d", n.ID),
				ThreadID:   &threadID,
				Author:     author,
				Body:       n.Body,
			}

			if createdAt != nil {
				discussion.CreatedAt = *createdAt
			}

			discussions = append(discussions, discussion)
		}
	}

	return discussions
}

func (s *gitLabIssueTrackerService) mapToIssue(gitlabIssue *gitlab.Issue) *model.Issue {
	var labels []string
	for _, l := range gitlabIssue.Labels {
		labels = append(labels, l)
	}

	var assignees []string
	for _, a := range gitlabIssue.Assignees {
		if a != nil {
			assignees = append(assignees, a.Username)
		}
	}

	var reporter *string
	if gitlabIssue.Author != nil {
		reporter = &gitlabIssue.Author.Username
	}

	return &model.Issue{
		Title:            &gitlabIssue.Title,
		Description:      &gitlabIssue.Description,
		Labels:           labels,
		Assignees:        assignees,
		Reporter:         reporter,
		ExternalIssueURL: &gitlabIssue.WebURL,
	}
}
