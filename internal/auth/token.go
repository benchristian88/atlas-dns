package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

const tokenBytes = 32

type TokenManager struct {
	secret []byte
}

func NewTokenManager(secret []byte) (*TokenManager, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("session secret must contain at least 32 bytes")
	}
	return &TokenManager{secret: append([]byte(nil), secret...)}, nil
}

func (m *TokenManager) NewSessionToken() (string, []byte, error) {
	return m.newToken("session")
}

func (m *TokenManager) NewCSRFToken() (string, []byte, error) {
	return m.newToken("csrf")
}

func (m *TokenManager) NewMFAChallengeToken() (string, []byte, error) {
	return m.newToken("mfa-challenge")
}

func (m *TokenManager) HashSessionToken(token string) []byte {
	return m.hash("session", token)
}

func (m *TokenManager) HashCSRFToken(token string) []byte {
	return m.hash("csrf", token)
}

func (m *TokenManager) HashMFAChallengeToken(token string) []byte {
	return m.hash("mfa-challenge", token)
}

func (m *TokenManager) HashRecoveryCode(code string) []byte {
	// Recovery codes contain 80 bits of CSPRNG entropy, so a purpose-separated
	// SHA-256 digest is offline-guess resistant without binding durable hashes to
	// the installation's independently managed session secret. This preserves
	// recovery codes across a database/credential-key restore while session and
	// challenge tokens remain keyed HMACs.
	digest := sha256.New()
	digest.Write([]byte("mfa-recovery-code"))
	digest.Write([]byte{0})
	digest.Write([]byte(code))
	return digest.Sum(nil)
}

func (m *TokenManager) Equal(left, right []byte) bool {
	return hmac.Equal(left, right)
}

func (m *TokenManager) newToken(purpose string) (string, []byte, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("generate %s token: %w", purpose, err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, m.hash(purpose, token), nil
}

func (m *TokenManager) hash(purpose, token string) []byte {
	digest := hmac.New(sha256.New, m.secret)
	digest.Write([]byte(purpose))
	digest.Write([]byte{0})
	digest.Write([]byte(token))
	return digest.Sum(nil)
}
