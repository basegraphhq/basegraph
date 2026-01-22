package model

import "time"

type Session struct {
	CreatedAt       time.Time `json:"created_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	ID              int64     `json:"id"`
	UserID          int64     `json:"user_id"`
	WorkOSSessionID *string   `json:"workos_session_id,omitempty"`
}
