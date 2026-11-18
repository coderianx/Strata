// Package repository provides database queries for Strata entities.
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/coderianx/Strata/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrDuplicateUser is returned by InsertNewUser when the username or email
// is already taken, so handlers can map it to a 409 Conflict response.
var ErrDuplicateUser = errors.New("repository: username or email is already taken")

// InsertNewUser stores a new user and returns the generated id. Only the
// bcrypt hash is written; the plaintext password never reaches this layer.
func InsertNewUser(
	ctx context.Context,
	name string,
	username string,
	email string,
	hashedPassword string,
) (int64, error) {
	var id int64

	err := store.DB.QueryRow(
		ctx,
		`
		INSERT INTO users(name, username, email, password)
		VALUES($1, $2, $3, $4)
		RETURNING id;
		`,
		name,
		username,
		email,
		hashedPassword,
	).Scan(&id)

	if err != nil {
		var pgErr *pgconn.PgError

		// 23505 is the PostgreSQL unique_violation SQLSTATE code.
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return 0, ErrDuplicateUser
		}

		return 0, fmt.Errorf("repository: insert new user: %w", err)
	}

	return id, nil
}

// ErrUserNotFound is returned when no user matches the query, so handlers
// can answer with the same 401 as a wrong password without leaking which
// part of the credentials was incorrect.
var ErrUserNotFound = errors.New("repository: user not found")

// GetUserByEmail returns the id, username and stored password hash of the
// user with the given email.
func GetUserByEmail(
	ctx context.Context,
	email string,
) (int64, string, string, error) {
	var (
		userID       int64
		username     string
		passwordHash string
	)

	err := store.DB.QueryRow(
		ctx,
		`
		SELECT id, username, password
		FROM users
		WHERE email = $1;
		`,
		email,
	).Scan(&userID, &username, &passwordHash)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, "", "", ErrUserNotFound
		}

		return 0, "", "", fmt.Errorf("repository: get user by email: %w", err)
	}

	return userID, username, passwordHash, nil
}

// InsertRefreshToken stores the SHA-256 hash of a refresh token together
// with its expiry. Tokens are stored hashed so a database leak cannot be
// replayed against the API.
func InsertRefreshToken(
	ctx context.Context,
	userID int64,
	tokenHash string,
	expiresAt time.Time,
) error {
	_, err := store.DB.Exec(
		ctx,
		`
		INSERT INTO refresh_tokens(user_id, token_hash, expires_at)
		VALUES($1, $2, $3);
		`,
		userID,
		tokenHash,
		expiresAt,
	)

	if err != nil {
		return fmt.Errorf("repository: insert refresh token: %w", err)
	}

	return nil
}

// ErrRefreshTokenInvalid is returned when a refresh token does not exist
// or has already expired; callers cannot distinguish the two on purpose.
var ErrRefreshTokenInvalid = errors.New("repository: refresh token is invalid or expired")

// ConsumeRefreshToken deletes a valid refresh token and returns the user
// it belonged to. Doing the delete and the read in one statement makes the
// token single-use: two concurrent requests cannot both succeed, and a
// consumed token can never be replayed even if the later steps fail.
func ConsumeRefreshToken(
	ctx context.Context,
	tokenHash string,
) (int64, error) {
	var userID int64

	err := store.DB.QueryRow(
		ctx,
		`
		DELETE FROM refresh_tokens
		WHERE token_hash = $1 AND expires_at > NOW()
		RETURNING user_id;
		`,
		tokenHash,
	).Scan(&userID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrRefreshTokenInvalid
		}

		return 0, fmt.Errorf("repository: consume refresh token: %w", err)
	}

	return userID, nil
}

// GetUsernameByID returns the username of the user with the given id.
func GetUsernameByID(
	ctx context.Context,
	userID int64,
) (string, error) {
	var username string

	err := store.DB.QueryRow(
		ctx,
		`
		SELECT username FROM users WHERE id = $1;
		`,
		userID,
	).Scan(&username)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrUserNotFound
		}

		return "", fmt.Errorf("repository: get username by id: %w", err)
	}

	return username, nil
}
