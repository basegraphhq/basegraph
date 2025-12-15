package dto

import (
	"time"

	"basegraph.app/api-server/internal/model"
)

type CreateOrganizationRequest struct {
	Slug        *string `json:"slug,omitempty" binding:"omitempty,min=1,max=255"`
	Name        string  `json:"name" binding:"required,min=1,max=255"`
	AdminUserID int64   `json:"admin_user_id,string" binding:"required"`
}

type OrganizationResponse struct {
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	ID          int64     `json:"id,string"`
	AdminUserID int64     `json:"admin_user_id,string"`
}

func ToOrganizationResponse(org *model.Organization) *OrganizationResponse {
	return &OrganizationResponse{
		ID:          org.ID,
		Name:        org.Name,
		Slug:        org.Slug,
		AdminUserID: org.AdminUserID,
		CreatedAt:   org.CreatedAt,
		UpdatedAt:   org.UpdatedAt,
	}
}
