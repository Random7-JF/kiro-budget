package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// User is an application account.
type User struct {
	ID       int64
	Username string
}

var (
	// ErrUserNotFound is returned when no user matches a lookup.
	ErrUserNotFound = errors.New("store: user not found")
	// ErrNoSession is returned when a session token is absent or expired.
	ErrNoSession = errors.New("store: session not found or expired")
)

// EnsureUser inserts a user with the given username and password hash if one
// does not already exist, and returns the (existing or newly created) user. The
// password hash of an existing user is left unchanged.
func (r *Repo) EnsureUser(ctx context.Context, username, passwordHash string) (User, error) {
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash) VALUES (?, ?)
		 ON CONFLICT(username) DO NOTHING`, username, passwordHash); err != nil {
		return User{}, fmt.Errorf("store: ensure user: %w", err)
	}
	u, _, err := r.UserByUsername(ctx, username)
	return u, err
}

// UserByUsername returns the user and password hash for a username, or
// ErrUserNotFound.
func (r *Repo) UserByUsername(ctx context.Context, username string) (User, string, error) {
	var (
		u    User
		hash string
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash FROM users WHERE username = ? COLLATE NOCASE`, username).
		Scan(&u.ID, &u.Username, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, "", ErrUserNotFound
	}
	if err != nil {
		return User{}, "", fmt.Errorf("store: user by username: %w", err)
	}
	return u, hash, nil
}

// UserByID returns the user with the given id, or ErrUserNotFound.
func (r *Repo) UserByID(ctx context.Context, id int64) (User, error) {
	var u User
	err := r.db.QueryRowContext(ctx,
		`SELECT id, username FROM users WHERE id = ?`, id).Scan(&u.ID, &u.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("store: user by id: %w", err)
	}
	return u, nil
}

// CreateSession stores a session token for a user with an expiry.
func (r *Repo) CreateSession(ctx context.Context, token string, userID int64, expiresAt time.Time) error {
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)`,
		token, userID, expiresAt.UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("store: create session: %w", err)
	}
	return nil
}

// SessionUser returns the user for a valid, unexpired session token. Expired or
// unknown tokens yield ErrNoSession (expired tokens are also deleted).
func (r *Repo) SessionUser(ctx context.Context, token string) (User, error) {
	var (
		u          User
		expiresStr string
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT u.id, u.username, s.expires_at
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token = ?`, token).Scan(&u.ID, &u.Username, &expiresStr)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNoSession
	}
	if err != nil {
		return User{}, fmt.Errorf("store: session user: %w", err)
	}

	expires, perr := time.Parse(time.RFC3339, expiresStr)
	if perr != nil || time.Now().After(expires) {
		_ = r.DeleteSession(ctx, token)
		return User{}, ErrNoSession
	}
	return u, nil
}

// DeleteSession removes a session token (used on logout). Removing a
// non-existent token is a no-op.
func (r *Repo) DeleteSession(ctx context.Context, token string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token); err != nil {
		return fmt.Errorf("store: delete session: %w", err)
	}
	return nil
}
