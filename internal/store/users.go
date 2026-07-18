package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// User is an application account.
type User struct {
	ID        int64
	Username  string
	FirstName string
	LastName  string
	Email     string
}

// DisplayName returns the user's full name if available, otherwise the username.
func (u User) DisplayName() string {
	if u.FirstName != "" || u.LastName != "" {
		full := u.FirstName
		if u.LastName != "" {
			if full != "" {
				full += " "
			}
			full += u.LastName
		}
		return full
	}
	return u.Username
}

var (
	// ErrUserNotFound is returned when no user matches a lookup.
	ErrUserNotFound = errors.New("store: user not found")
	// ErrNoSession is returned when a session token is absent or expired.
	ErrNoSession = errors.New("store: session not found or expired")
	// ErrUserExists is returned when attempting to create a user with a
	// username that is already taken.
	ErrUserExists = errors.New("store: username already taken")
)

// EnsureUser inserts a user with the given username and password hash if one
// does not already exist, and returns the (existing or newly created) user. The
// password hash of an existing user is left unchanged. Profile fields are not
// set by this function (it is used only for the demo user).
func (r *Repo) EnsureUser(ctx context.Context, username, passwordHash string) (User, error) {
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash) VALUES (?, ?)
		 ON CONFLICT(username) DO NOTHING`, username, passwordHash); err != nil {
		return User{}, fmt.Errorf("store: ensure user: %w", err)
	}
	u, _, err := r.UserByUsername(ctx, username)
	return u, err
}

// CreateUser inserts a new user with full profile fields and returns the stored
// user. Returns an error (wrapping ErrUserExists) if the username is taken.
func (r *Repo) CreateUser(ctx context.Context, username, passwordHash, firstName, lastName, email string) (User, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, first_name, last_name, email)
		 VALUES (?, ?, ?, ?, ?)`,
		username, passwordHash, firstName, lastName, email)
	if err != nil {
		// SQLite UNIQUE constraint violation
		if isUniqueConstraint(err) {
			return User{}, ErrUserExists
		}
		return User{}, fmt.Errorf("store: create user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, fmt.Errorf("store: create user last id: %w", err)
	}
	return User{ID: id, Username: username, FirstName: firstName, LastName: lastName, Email: email}, nil
}

// UserByUsername returns the user and password hash for a username, or
// ErrUserNotFound.
func (r *Repo) UserByUsername(ctx context.Context, username string) (User, string, error) {
	var (
		u    User
		hash string
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, first_name, last_name, email FROM users WHERE username = ? COLLATE NOCASE`, username).
		Scan(&u.ID, &u.Username, &hash, &u.FirstName, &u.LastName, &u.Email)
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
		`SELECT id, username, first_name, last_name, email FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Username, &u.FirstName, &u.LastName, &u.Email)
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
		`SELECT u.id, u.username, u.first_name, u.last_name, u.email, s.expires_at
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token = ?`, token).
		Scan(&u.ID, &u.Username, &u.FirstName, &u.LastName, &u.Email, &expiresStr)
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

// isUniqueConstraint reports whether err is a SQLite UNIQUE constraint
// violation (modernc.org/sqlite always includes "UNIQUE constraint failed" in
// the error message for such violations).
func isUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
