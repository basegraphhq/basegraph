package model

import "time"

type CredentialType string

const (
	CredentialTypeUserOAuth  CredentialType = "user_oauth" //nolint:gosec // False positive: enum constant, not hardcoded credential
	CredentialTypeBot        CredentialType = "bot"
	CredentialTypeAppInstall CredentialType = "app_installation"
	CredentialTypeAPIKey     CredentialType = "api_key"
)

type IntegrationCredential struct {
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	UserID         *int64         `json:"user_id,omitempty"`
	RefreshToken   *string        `json:"-"`
	TokenExpiresAt *time.Time     `json:"-"`
	RevokedAt      *time.Time     `json:"revoked_at,omitempty"`
	CredentialType CredentialType `json:"credential_type"`
	AccessToken    string         `json:"-"`
	Scopes         []string       `json:"scopes,omitempty"`
	ID             int64          `json:"id"`
	IntegrationID  int64          `json:"integration_id"`
	IsPrimary      bool           `json:"is_primary"`
}
