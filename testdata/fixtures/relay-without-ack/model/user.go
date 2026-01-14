package model

import "time"

type User struct {
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	AvatarURL *string   `json:"avatar_url,omitempty"`
	WorkOSID  *string   `json:"workos_id,omitempty"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	ID        int64     `json:"id"`
}
