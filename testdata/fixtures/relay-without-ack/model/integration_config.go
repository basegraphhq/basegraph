package model

import (
	"encoding/json"
	"time"
)

// IntegrationConfig stores per-integration configuration values such as webhook metadata.
type IntegrationConfig struct {
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	Key           string          `json:"key"`
	ConfigType    string          `json:"config_type"`
	Value         json.RawMessage `json:"value"`
	ID            int64           `json:"id"`
	IntegrationID int64           `json:"integration_id"`
}

// Config key constants
const (
	ConfigKeyServiceAccount = "service_account"
)

// ServiceAccountConfig represents the bot/service account identity for an integration.
// Stored as IntegrationConfig with key="service_account" and config_type="identity".
type ServiceAccountConfig struct {
	Username string `json:"username"`
	UserID   int64  `json:"user_id"`
}
