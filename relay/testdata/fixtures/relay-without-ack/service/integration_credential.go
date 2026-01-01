package service

import (
	"context"
	"fmt"

	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/store"
)

type IntegrationCredentialService interface {
	GetWebhookSecret(ctx context.Context, integrationID int64) (string, error)
	ValidateWebhookToken(ctx context.Context, integrationID int64, token string) error
}

type integrationCredentialService struct {
	credentialStore store.IntegrationCredentialStore
}

func NewIntegrationCredentialService(credentialStore store.IntegrationCredentialStore) IntegrationCredentialService {
	return &integrationCredentialService{
		credentialStore: credentialStore,
	}
}

func (s *integrationCredentialService) GetWebhookSecret(ctx context.Context, integrationID int64) (string, error) {
	creds, err := s.credentialStore.ListActiveByIntegration(ctx, integrationID)
	if err != nil {
		return "", fmt.Errorf("listing active credentials: %w", err)
	}

	for _, cred := range creds {
		if cred.CredentialType == model.CredentialTypeWebhookSecret {
			return cred.AccessToken, nil
		}
	}

	return "", fmt.Errorf("webhook secret not found")
}

func (s *integrationCredentialService) ValidateWebhookToken(ctx context.Context, integrationID int64, token string) error {
	webhookSecret, err := s.GetWebhookSecret(ctx, integrationID)
	if err != nil {
		return fmt.Errorf("invalid webhook token")
	}

	if webhookSecret != token {
		return fmt.Errorf("invalid webhook token")
	}

	return nil
}
