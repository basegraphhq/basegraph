package model

import "time"

type Organization struct {
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	ID          int64     `json:"id"`
	AdminUserID int64     `json:"admin_user_id"`
	IsDeleted   bool      `json:"-"`
}
