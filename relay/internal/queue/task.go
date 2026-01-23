package queue

import "fmt"

type TaskType string

const (
	TaskTypeIssueEvent     TaskType = "issue_event"
	TaskTypeWorkspaceSetup TaskType = "workspace_setup"
	TaskTypeRepoSync       TaskType = "repo_sync"
)

type Task struct {
	TaskType        TaskType
	EventLogID      int64
	IssueID         int64
	EventType       string
	TraceID         *string
	Attempt         int
	TriggerThreadID string

	WorkspaceID    *int64
	OrganizationID *int64
	RunID          *int64
	RepoID         *int64
	Branch         string
}

func WorkspaceStreamName(orgID, workspaceID int64) string {
	return fmt.Sprintf("agent-stream:org-%d:workspace-%d", orgID, workspaceID)
}
