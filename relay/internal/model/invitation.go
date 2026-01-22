package model

import "time"

type InvitationStatus string

const (
	InvitationStatusPending  InvitationStatus = "pending"
	InvitationStatusAccepted InvitationStatus = "accepted"
	InvitationStatusExpired  InvitationStatus = "expired"
	InvitationStatusRevoked  InvitationStatus = "revoked"
)

type Invitation struct {
	ID         int64            `json:"id"`
	Email      string           `json:"email"`
	Token      string           `json:"token"`
	Status     InvitationStatus `json:"status"`
	InvitedBy  *int64           `json:"invited_by,omitempty"`
	AcceptedBy *int64           `json:"accepted_by,omitempty"`
	ExpiresAt  time.Time        `json:"expires_at"`
	CreatedAt  time.Time        `json:"created_at"`
	AcceptedAt *time.Time       `json:"accepted_at,omitempty"`
}

func (i *Invitation) IsValid() bool {
	return i.Status == InvitationStatusPending && time.Now().Before(i.ExpiresAt)
}
