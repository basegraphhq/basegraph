package llm

import (
	"basegraph.app/relay/internal/domain"
)

type KeywordRequest struct {
	Issue    *domain.Issue
	Text     string
	Event    domain.Event
	Existing []domain.Keyword
}

type GapRequest struct {
	Issue   *domain.Issue
	Event   domain.Event
	Context domain.ContextSnapshot
}

type SpecRequest struct {
	Issue   *domain.Issue
	Context domain.ContextSnapshot
	Gaps    []domain.Gap
}
