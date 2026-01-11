package model

import "time"

// ConversationMessage represents a provider-agnostic message in a conversation.
// This abstraction works across GitLab, GitHub, Slack, and future providers.
type ConversationMessage struct {
	Seq        int    // 1, 2, 3... (position in conversation)
	Author     string // username (normalized across providers)
	Role       string // reporter | assignee | self | other
	Timestamp  time.Time
	ReplyToSeq *int   // parent message seq (nil if top-level)
	Content    string // markdown/text body

	// Relay annotations (computed at handoff, not from provider)
	AnswersGapID *int64 // if this message likely answered a gap
	IsProceed    bool   // if this contains proceed signal
}

// Conversation role constants.
const (
	RoleReporter = "reporter"
	RoleAssignee = "assignee"
	RoleSelf     = "self"
	RoleOther    = "other"
)
