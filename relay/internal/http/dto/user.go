package dto

import (
	"time"

	"basegraph.co/relay/internal/model"
)

type CreateUserRequest struct {
	AvatarURL *string `json:"avatar_url,omitempty" binding:"omitempty,url,max=2048"`
	Name      string  `json:"name" binding:"required,min=1,max=255"`
	Email     string  `json:"email" binding:"required,email,max=255"`
}

type UserResponse struct {
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	AvatarURL *string   `json:"avatar_url,omitempty"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	ID        int64     `json:"id,string"`
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
	AvatarURL *string `json:"avatar_url,omitempty" binding:"omitempty,url,max=2048"`
	Name      string  `json:"name" binding:"required,min=1,max=255"`
	Email     string  `json:"email" binding:"required,email,max=255"`
}

type SyncUserResponse struct {
	User            *UserResponse       `json:"user"`
	Organizations   []OrganizationBrief `json:"organizations"`
	HasOrganization bool                `json:"has_organization"`
}

type OrganizationBrief struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
	ID   int64  `json:"id,string"`
}

func ToOrganizationBrief(org model.Organization) OrganizationBrief {
	return OrganizationBrief{
		ID:   org.ID,
		Name: org.Name,
		Slug: org.Slug,
	}
}
