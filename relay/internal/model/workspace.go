package model

import "time"

type Workspace struct {
	ID             int64     `json:"id"`
	AdminUserID    int64     `json:"admin_user_id"`
	OrganizationID int64     `json:"organization_id"`
	UserID         int64     `json:"user_id"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	Description    *string   `json:"description,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	IsDeleted      bool      `json:"-"` // internal, not exposed in API
}
