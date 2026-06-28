package auth_test

import (
	"errors"
	"testing"

	"gopherledger/internal/auth"
)

func TestGenerateAndValidateToken(t *testing.T) {
	userID := int64(42)

	token, err := auth.GenerateToken(userID)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	if token == "" {
		t.Error("expected non-empty token")
	}

	gotID, err := auth.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if gotID != userID {
		t.Errorf("expected userID %d, got %d", userID, gotID)
	}
}

func TestValidateToken_Invalid(t *testing.T) {
	_, err := auth.ValidateToken("invalid-token")
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got: %v", err)
	}
}

func TestValidateToken_Tampered(t *testing.T) {
	token, _ := auth.GenerateToken(1)

	// Подделываем токен (меняем последний символ)
	tampered := token[:len(token)-1] + "X"

	_, err := auth.ValidateToken(tampered)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken for tampered token, got: %v", err)
	}
}
