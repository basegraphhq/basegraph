package model

import "time"

type Workspace struct {
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	Description    *string    `json:"description,omitempty"`
	Name           string     `json:"name"`
	Slug           string     `json:"slug"`
	ID             int64      `json:"id"`
	AdminUserID    int64      `json:"admin_user_id"`
	OrganizationID int64      `json:"organization_id"`
	UserID         int64      `json:"user_id"`
	IsDeleted      bool       `json:"-"`
	RepoReadyAt    *time.Time `json:"repo_ready_at,omitempty"`
}
