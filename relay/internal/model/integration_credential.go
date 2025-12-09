package model

import "time"

type CredentialType string

const (
	CredentialTypeUserOAuth  CredentialType = "user_oauth"
	CredentialTypeBot        CredentialType = "bot"
	CredentialTypeAppInstall CredentialType = "app_installation"
	CredentialTypeAPIKey     CredentialType = "api_key"
)

type IntegrationCredential struct {
	ID             int64          `json:"id"`
	IntegrationID  int64          `json:"integration_id"`
	UserID         *int64         `json:"user_id,omitempty"`
	CredentialType CredentialType `json:"credential_type"`
	AccessToken    string         `json:"-"`
	RefreshToken   *string        `json:"-"`
	TokenExpiresAt *time.Time     `json:"-"`
	Scopes         []string       `json:"scopes,omitempty"`
	IsPrimary      bool           `json:"is_primary"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	RevokedAt      *time.Time     `json:"revoked_at,omitempty"`
}
