// Package auth provides password hashing, session-token generation, and the
// demo-user bootstrap shared by the server and the admin CLI.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"github.com/budget-tracker/budget-tracker/internal/store"
)

// Demo account credentials, created automatically at startup for local use.
const (
	DemoUsername = "test"
	DemoPassword = "password123"
)

// HashPassword returns a bcrypt hash of the given plaintext password.
func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("auth: hash password: %w", err)
	}
	return string(h), nil
}

// CheckPassword reports whether the plaintext password matches the bcrypt hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// NewToken returns a cryptographically random session token (256 bits, hex).
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// EnsureDemoUser creates the demo user (test / password123) if it does not
// already exist, returning it. Intended for local development and testing.
func EnsureDemoUser(ctx context.Context, repo *store.Repo) (store.User, error) {
	hash, err := HashPassword(DemoPassword)
	if err != nil {
		return store.User{}, err
	}
	return repo.EnsureUser(ctx, DemoUsername, hash)
}
