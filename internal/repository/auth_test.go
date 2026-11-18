//go:build integration

// Database tests for the auth repository. They run only when a
// disposable PostgreSQL instance is configured:
//
//	TEST_DATABASE_URL=postgres://user:pass@localhost/strata_test \
//		go test -tags=integration ./...
//
// Every test truncates the users and refresh_tokens tables, so the
// database must not hold real data.

package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/coderianx/Strata/internal/store"
	"golang.org/x/crypto/bcrypt"
)

const testDBEnv = "TEST_DATABASE_URL"

func TestMain(m *testing.M) {
	if dsn := os.Getenv(testDBEnv); dsn != "" {
		os.Setenv("DB_URL", dsn)

		if err := store.ConnectDatabase(); err != nil {
			fmt.Fprintf(os.Stderr, "integration: connect database: %v\n", err)
			os.Exit(1)
		}

		if err := store.CreateTables(); err != nil {
			fmt.Fprintf(os.Stderr, "integration: create tables: %v\n", err)
			os.Exit(1)
		}
	}

	code := m.Run()

	if store.DB != nil {
		store.DB.Close()
	}

	os.Exit(code)
}

// requireDB skips database tests when no test instance is configured
// and resets the tables so tests cannot see each other's rows.
func requireDB(t *testing.T) {
	t.Helper()

	if store.DB == nil {
		t.Skipf("%s is not set, skipping database test", testDBEnv)
	}

	_, err := store.DB.Exec(
		context.Background(),
		`TRUNCATE users, organizations, organization_members, organization_invitations, refresh_tokens RESTART IDENTITY CASCADE`,
	)
	if err != nil {
		t.Fatalf("reset tables: %v", err)
	}
}

// hashPassword hashes a fixture password with bcrypt's minimum cost:
// fixture strength is irrelevant, test speed is not.
func hashPassword(t *testing.T, password string) string {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash fixture password: %v", err)
	}

	return string(hash)
}

// insertUser stores a user through the public repository API and
// returns their id.
func insertUser(t *testing.T, username, email string) int64 {
	t.Helper()

	id, err := InsertNewUser(
		context.Background(),
		"Test User",
		username,
		email,
		hashPassword(t, "sifre1234"),
	)
	if err != nil {
		t.Fatalf("insert fixture user: %v", err)
	}

	return id
}

// tokenHash mirrors the SHA-256 hex digest the handlers store.
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))

	return hex.EncodeToString(sum[:])
}

func TestInsertNewUser(t *testing.T) {
	requireDB(t)

	t.Run("stores the user", func(t *testing.T) {
		id, err := InsertNewUser(
			context.Background(),
			"Kodcu",
			"kodcu_42",
			"kodcu@strata.dev",
			hashPassword(t, "sifre1234"),
		)
		if err != nil {
			t.Fatalf("InsertNewUser() = %v, want nil", err)
		}

		if id <= 0 {
			t.Errorf("InsertNewUser() id = %d, want a positive id", id)
		}
	})

	t.Run("rejects a duplicate username", func(t *testing.T) {
		insertUser(t, "dup_user", "dup@strata.dev")

		_, err := InsertNewUser(
			context.Background(),
			"Other",
			"dup_user",
			"other@strata.dev",
			hashPassword(t, "sifre1234"),
		)
		if !errors.Is(err, ErrDuplicateUser) {
			t.Errorf("InsertNewUser() = %v, want ErrDuplicateUser", err)
		}
	})

	t.Run("rejects a duplicate email", func(t *testing.T) {
		insertUser(t, "dup_email_user", "dupemail@strata.dev")

		_, err := InsertNewUser(
			context.Background(),
			"Other",
			"other_user",
			"dupemail@strata.dev",
			hashPassword(t, "sifre1234"),
		)
		if !errors.Is(err, ErrDuplicateUser) {
			t.Errorf("InsertNewUser() = %v, want ErrDuplicateUser", err)
		}
	})
}

func TestGetUserByEmail(t *testing.T) {
	requireDB(t)

	t.Run("returns the stored user", func(t *testing.T) {
		hash := hashPassword(t, "sifre1234")

		id, err := InsertNewUser(
			context.Background(),
			"Kodcu",
			"kodcu_42",
			"kodcu@strata.dev",
			hash,
		)
		if err != nil {
			t.Fatalf("insert fixture user: %v", err)
		}

		gotID, username, gotHash, err := GetUserByEmail(
			context.Background(),
			"kodcu@strata.dev",
		)
		if err != nil {
			t.Fatalf("GetUserByEmail() = %v, want nil", err)
		}

		if gotID != id {
			t.Errorf("id = %d, want %d", gotID, id)
		}

		if username != "kodcu_42" {
			t.Errorf("username = %q, want %q", username, "kodcu_42")
		}

		if gotHash != hash {
			t.Errorf("password hash = %q, want %q", gotHash, hash)
		}

		if err := bcrypt.CompareHashAndPassword(
			[]byte(gotHash),
			[]byte("sifre1234"),
		); err != nil {
			t.Errorf("stored hash does not verify: %v", err)
		}
	})

	t.Run("unknown email", func(t *testing.T) {
		_, _, _, err := GetUserByEmail(
			context.Background(),
			"missing@strata.dev",
		)
		if !errors.Is(err, ErrUserNotFound) {
			t.Errorf("GetUserByEmail() = %v, want ErrUserNotFound", err)
		}
	})
}

func TestInsertRefreshToken(t *testing.T) {
	requireDB(t)

	userID := insertUser(t, "kodcu_42", "kodcu@strata.dev")

	t.Run("stores a token", func(t *testing.T) {
		err := InsertRefreshToken(
			context.Background(),
			userID,
			tokenHash("token-insert"),
			time.Now().Add(time.Hour),
		)
		if err != nil {
			t.Errorf("InsertRefreshToken() = %v, want nil", err)
		}
	})

	t.Run("rejects a duplicate token hash", func(t *testing.T) {
		hash := tokenHash("token-duplicate")
		expires := time.Now().Add(time.Hour)

		if err := InsertRefreshToken(
			context.Background(),
			userID,
			hash,
			expires,
		); err != nil {
			t.Fatalf("first InsertRefreshToken() = %v, want nil", err)
		}

		err := InsertRefreshToken(context.Background(), userID, hash, expires)
		if err == nil {
			t.Error("second InsertRefreshToken() = nil, want a unique violation")
		}
	})
}

func TestConsumeRefreshToken(t *testing.T) {
	requireDB(t)

	userID := insertUser(t, "kodcu_42", "kodcu@strata.dev")

	t.Run("returns the owner of a valid token", func(t *testing.T) {
		if err := InsertRefreshToken(
			context.Background(),
			userID,
			tokenHash("token-valid"),
			time.Now().Add(time.Hour),
		); err != nil {
			t.Fatalf("insert fixture token: %v", err)
		}

		got, err := ConsumeRefreshToken(context.Background(), tokenHash("token-valid"))
		if err != nil {
			t.Fatalf("ConsumeRefreshToken() = %v, want nil", err)
		}

		if got != userID {
			t.Errorf("user id = %d, want %d", got, userID)
		}
	})

	t.Run("a consumed token cannot be used again", func(t *testing.T) {
		if err := InsertRefreshToken(
			context.Background(),
			userID,
			tokenHash("token-single-use"),
			time.Now().Add(time.Hour),
		); err != nil {
			t.Fatalf("insert fixture token: %v", err)
		}

		if _, err := ConsumeRefreshToken(
			context.Background(),
			tokenHash("token-single-use"),
		); err != nil {
			t.Fatalf("first ConsumeRefreshToken() = %v, want nil", err)
		}

		_, err := ConsumeRefreshToken(context.Background(), tokenHash("token-single-use"))
		if !errors.Is(err, ErrRefreshTokenInvalid) {
			t.Errorf("second ConsumeRefreshToken() = %v, want ErrRefreshTokenInvalid", err)
		}
	})

	t.Run("expired token", func(t *testing.T) {
		if err := InsertRefreshToken(
			context.Background(),
			userID,
			tokenHash("token-expired"),
			time.Now().Add(-time.Hour),
		); err != nil {
			t.Fatalf("insert fixture token: %v", err)
		}

		_, err := ConsumeRefreshToken(context.Background(), tokenHash("token-expired"))
		if !errors.Is(err, ErrRefreshTokenInvalid) {
			t.Errorf("ConsumeRefreshToken() = %v, want ErrRefreshTokenInvalid", err)
		}
	})

	t.Run("unknown token", func(t *testing.T) {
		_, err := ConsumeRefreshToken(context.Background(), tokenHash("never-issued"))
		if !errors.Is(err, ErrRefreshTokenInvalid) {
			t.Errorf("ConsumeRefreshToken() = %v, want ErrRefreshTokenInvalid", err)
		}
	})
}

func TestGetUsernameByID(t *testing.T) {
	requireDB(t)

	t.Run("returns the username", func(t *testing.T) {
		id := insertUser(t, "kodcu_42", "kodcu@strata.dev")

		username, err := GetUsernameByID(context.Background(), id)
		if err != nil {
			t.Fatalf("GetUsernameByID() = %v, want nil", err)
		}

		if username != "kodcu_42" {
			t.Errorf("username = %q, want %q", username, "kodcu_42")
		}
	})

	t.Run("unknown id", func(t *testing.T) {
		_, err := GetUsernameByID(context.Background(), 424242)
		if !errors.Is(err, ErrUserNotFound) {
			t.Errorf("GetUsernameByID() = %v, want ErrUserNotFound", err)
		}
	})
}
