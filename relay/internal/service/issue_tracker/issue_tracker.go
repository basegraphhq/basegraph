package issue_tracker

import (
	"context"

	"basegraph.app/relay/internal/model"
)

type FetchIssueParams struct {
	IntegrationID int64 // needed to instantiate provider api client. eg: gitlab self-hosted
	ProjectID     int64
	IssueIID      int64
}

type FetchDiscussionsParams struct {
	IntegrationID int64
	ProjectID     int64
	IssueIID      int64
}

type IsReplyParams struct {
	IntegrationID int64
	ProjectID     int64
	IssueIID      int64
	DiscussionID  string // GitLab-specific thread ID
	CommentID     string // Provider-specific comment ID
	UserID        int64  // Check if replying to this user. Mostly, we'll use this for relay
}

type Discussion struct {
	ID    string
	Notes []Note
}

type Note struct {
	ID       int64
	AuthorID int64
	Body     string
}

type IssueTrackerService interface {
	FetchIssue(ctx context.Context, params FetchIssueParams) (*model.Issue, error)
	FetchDiscussions(ctx context.Context, params FetchDiscussionsParams) ([]Discussion, error)
	IsReplyToUser(ctx context.Context, params IsReplyParams) (bool, error)
}
