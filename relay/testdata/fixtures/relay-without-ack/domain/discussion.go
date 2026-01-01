package domain

import "time"

// DiscussionType represents the intent of a discussion entry.
type DiscussionType string

const (
	DiscussionTypeQuestion DiscussionType = "question"
	DiscussionTypeAnswer   DiscussionType = "answer"
	DiscussionTypeNote     DiscussionType = "note"
)

// DiscussionStatus represents lifecycle state of a discussion item.
type DiscussionStatus string

const (
	DiscussionStatusOpen      DiscussionStatus = "open"
	DiscussionStatusResolved  DiscussionStatus = "resolved"
	DiscussionStatusDismissed DiscussionStatus = "dismissed"
)

// Discussion captures tracked conversation elements (questions, answers, notes).
type Discussion struct {
	ID             string            `json:"id"`
	ExternalID     *string           `json:"external_id,omitempty"`
	ParentID       *string           `json:"parent_id,omitempty"`
	Type           DiscussionType    `json:"type"`
	Author         string            `json:"author"`
	Body           string            `json:"body"`
	Category       string            `json:"category,omitempty"`
	Severity       GapSeverity       `json:"severity,omitempty"`
	Status         DiscussionStatus  `json:"status"`
	AskedAt        time.Time         `json:"asked_at"`
	ResolvedAt     *time.Time        `json:"resolved_at,omitempty"`
	ResolvedBy     *string           `json:"resolved_by,omitempty"`
	RelatedNoteID  *string           `json:"related_note_id,omitempty"`
	FollowUpNeeded bool              `json:"follow_up_needed,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}
