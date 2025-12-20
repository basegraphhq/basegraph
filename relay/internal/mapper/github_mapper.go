package mapper

import (
	"context"
	"fmt"
)

type GitHubEventMapper struct{}

func NewGitHubEventMapper() *GitHubEventMapper {
	return &GitHubEventMapper{}
}

func (m *GitHubEventMapper) Map(ctx context.Context, body map[string]any, headers map[string]string) (CanonicalEventType, error) {
	return "", fmt.Errorf("github webhook mapper not implemented yet")
}
