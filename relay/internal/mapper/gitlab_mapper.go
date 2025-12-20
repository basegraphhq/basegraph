package mapper

import (
	"context"
	"fmt"
)

type GitLabEventMapper struct{}

func NewGitLabEventMapper() *GitLabEventMapper {
	return &GitLabEventMapper{}
}

func (m *GitLabEventMapper) Map(ctx context.Context, body map[string]any, headers map[string]string) (CanonicalEventType, error) {
	headerEventType := headers["X-Gitlab-Event"]

	objectKind := ""
	if ok, exists := body["object_kind"]; exists {
		objectKind, _ = ok.(string)
	}

	canonicalType := m.mapGitLabEvent(headerEventType, objectKind)
	if canonicalType == "" {
		return "", fmt.Errorf("unknown gitlab event type: header=%q object_kind=%q", headerEventType, objectKind)
	}

	return canonicalType, nil
}

func (m *GitLabEventMapper) mapGitLabEvent(headerEventType, objectKind string) CanonicalEventType {
	switch headerEventType {
	case "Issue Hook":
		return EventIssueCreated
	case "Note Hook":
		return EventReply
	case "Merge Request Hook":
		return EventPRCreated
	}

	switch objectKind {
	case "issue":
		return EventIssueCreated
	case "note":
		return EventReply
	case "merge_request":
		return EventPRCreated
	}

	return ""
}
