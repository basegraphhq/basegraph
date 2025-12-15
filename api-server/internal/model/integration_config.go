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
