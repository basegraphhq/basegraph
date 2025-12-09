package dto

import (
	"time"

	"basegraph.app/relay/internal/model"
)

type CreateUserRequest struct {
	Name      string  `json:"name" binding:"required,min=1,max=255"`
	Email     string  `json:"email" binding:"required,email,max=255"`
	AvatarURL *string `json:"avatar_url,omitempty" binding:"omitempty,url,max=2048"`
}

type UserResponse struct {
	ID        int64     `json:"id,string"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	AvatarURL *string   `json:"avatar_url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func ToUserResponse(u *model.User) *UserResponse {
	return &UserResponse{
		ID:        u.ID,
		Name:      u.Name,
		Email:     u.Email,
		AvatarURL: u.AvatarURL,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}

type SyncUserRequest struct {
	Name      string  `json:"name" binding:"required,min=1,max=255"`
	Email     string  `json:"email" binding:"required,email,max=255"`
	AvatarURL *string `json:"avatar_url,omitempty" binding:"omitempty,url,max=2048"`
}

type SyncUserResponse struct {
	User            *UserResponse       `json:"user"`
	Organizations   []OrganizationBrief `json:"organizations"`
	HasOrganization bool                `json:"has_organization"`
}

type OrganizationBrief struct {
	ID   int64  `json:"id,string"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func ToOrganizationBrief(org model.Organization) OrganizationBrief {
	return OrganizationBrief{
		ID:   org.ID,
		Name: org.Name,
		Slug: org.Slug,
	}
}
