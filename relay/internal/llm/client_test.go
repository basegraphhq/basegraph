package llm

import (
	"testing"
)

func TestClientWithoutAPIKey(t *testing.T) {
	client, err := NewClient("") // Should return error
	if err == nil {
		t.Fatal("Expected error for empty API key, got nil")
	}
	if client != nil {
		t.Fatal("Expected nil client for empty API key")
	}
}
