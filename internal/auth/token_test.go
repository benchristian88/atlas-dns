package auth

import (
	"bytes"
	"testing"
)

func TestTokenManagerUsesPurposeSeparatedHashes(t *testing.T) {
	t.Parallel()
	manager, err := NewTokenManager(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatalf("NewTokenManager() error = %v", err)
	}
	token, hash, err := manager.NewSessionToken()
	if err != nil {
		t.Fatalf("NewSessionToken() error = %v", err)
	}
	if !manager.Equal(hash, manager.HashSessionToken(token)) {
		t.Fatal("session token hash did not verify")
	}
	if manager.Equal(hash, manager.HashCSRFToken(token)) {
		t.Fatal("session token hash matched the CSRF purpose")
	}
}

func TestRecoveryCodeHashIsPurposeSeparatedAndPortableAcrossSessionSecretRotation(t *testing.T) {
	t.Parallel()
	first, err := NewTokenManager(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := NewTokenManager(bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatal(err)
	}
	const code = "0123456789abcdef0123"
	firstHash := first.HashRecoveryCode(code)
	if !bytes.Equal(firstHash, rotated.HashRecoveryCode(code)) {
		t.Fatal("session-secret rotation invalidated a durable recovery-code hash")
	}
	if bytes.Equal(firstHash, first.HashSessionToken(code)) || bytes.Contains(firstHash, []byte(code)) {
		t.Fatal("recovery-code hash was not one-way and purpose separated")
	}
}
