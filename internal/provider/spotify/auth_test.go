package spotify

import (
	"context"
	"testing"
)

func TestValidateCredentials_Empty(t *testing.T) {
	err := ValidateCredentials(context.Background(), "", "", "")
	if err == nil {
		t.Fatalf("expected error for empty credentials, got nil")
	}
}

func TestValidateCredentials_InvalidClient(t *testing.T) {
	err := ValidateCredentials(context.Background(), "invalid_client_id_12345", "invalid_client_secret_67890", "")
	if err == nil {
		t.Fatalf("expected error for fake client credentials, got nil")
	}
}

func TestValidateCredentials_InvalidToken(t *testing.T) {
	err := ValidateCredentials(context.Background(), "", "", "BQC_invalid_token_12345")
	if err == nil {
		t.Fatalf("expected error for fake bearer token, got nil")
	}
}
