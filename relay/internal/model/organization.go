package model

import "time"

type Organization struct {
	ID          int64     `json:"id"`
	AdminUserID int64     `json:"admin_user_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	IsDeleted   bool      `json:"-"` // internal, not exposed in API
}
