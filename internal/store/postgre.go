// Package store provides PostgreSQL data storage for Strata.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/coderianx/Strata/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB is the shared PostgreSQL connection pool, set by
// ConnectDatabase and read by the repository layer; it is nil before
// that.
var DB *pgxpool.Pool

// ConnectDatabase connects to the PostgreSQL instance named by the
// DB_URL environment variable and pings it with a 5 second timeout.
// On any failure the pool is closed and DB is left nil.
func ConnectDatabase() error {
	var err error

	dbURL := config.GetEnv("DB_URL")

	if len(dbURL) == 0 {
		return errors.New("store: DB_URL is not set in .env")
	}

	DB, err = pgxpool.New(
		context.Background(),
		dbURL,
	)

	if err != nil {
		return err
	}

	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := DB.Ping(pingCtx); err != nil {
		DB.Close()
		DB = nil
		return fmt.Errorf("store: ping database: %w", err)
	}

	return nil
}

// CreateTables creates every Strata table with IF NOT EXISTS, so it
// is safe to run on every startup. Existing tables are never
// rewritten; the email patch keeps databases created before the
// email column usable.
func CreateTables() error {

	_, err := DB.Exec(
		context.Background(),
		`
		CREATE TABLE IF NOT EXISTS users(
			id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			name TEXT NOT NULL,
			username TEXT NOT NULL UNIQUE,
			email TEXT NOT NULL UNIQUE,
			password TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		`,
	)

	if err != nil {
		return fmt.Errorf("store: create 'users' table error: %w", err)
	}

	// CREATE TABLE IF NOT EXISTS never modifies an already existing table,
	// so patch databases created before the email column was part of the
	// schema. The column is added as nullable because NOT NULL cannot be
	// enforced on pre-existing rows that have no email.
	_, err = DB.Exec(
		context.Background(),
		`
		ALTER TABLE users ADD COLUMN IF NOT EXISTS email TEXT;
		`,
	)

	if err != nil {
		return fmt.Errorf("store: add 'email' column to 'users' error: %w", err)
	}

	_, err = DB.Exec(
		context.Background(),
		`
		CREATE UNIQUE INDEX IF NOT EXISTS users_email_key ON users(email);
		`,
	)

	if err != nil {
		return fmt.Errorf("store: create 'users.email' unique index error: %w", err)
	}

	_, err = DB.Exec(
		context.Background(),
		`
		CREATE TABLE IF NOT EXISTS refresh_tokens(
			id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_hash CHAR(64) NOT NULL UNIQUE,
			expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		`,
	)

	if err != nil {
		return fmt.Errorf("store: create 'refresh_tokens' table error: %w", err)
	}

	_, err = DB.Exec(
		context.Background(),
		`
		CREATE TABLE IF NOT EXISTS organizations (
			id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			name TEXT NOT NULL,
			slug TEXT NOT NULL UNIQUE,
			description TEXT,
			logo_url TEXT,
			owner_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		`,
	)

	if err != nil {
		return fmt.Errorf("store: create 'organizations' table: %w", err)
	}

	_, err = DB.Exec(
		context.Background(),
		`
		CREATE TABLE IF NOT EXISTS organization_members  (
			id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
			user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			role TEXT NOT NULL DEFAULT 'member', -- owner or member
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

			UNIQUE (organization_id, user_id)
		);
		`,
	)

	if err != nil {
		return fmt.Errorf("store: create 'organization_members' table: %w", err)
	}

	_, err = DB.Exec(
		context.Background(),
		`
		CREATE TABLE IF NOT EXISTS organization_invitations (
		    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

		    organization_id BIGINT NOT NULL
		        REFERENCES organizations(id) ON DELETE CASCADE,

		    user_id BIGINT NOT NULL
		        REFERENCES users(id) ON DELETE CASCADE,

		    invited_by BIGINT NOT NULL
		        REFERENCES users(id) ON DELETE CASCADE,

		    role TEXT NOT NULL DEFAULT 'member',

		    status TEXT NOT NULL DEFAULT 'pending',

		    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE UNIQUE INDEX IF NOT EXISTS unique_pending_invitation
		ON organization_invitations (organization_id, user_id)
		WHERE status = 'pending';
		`,
	)

	if err != nil {
		return fmt.Errorf("store: create 'organization_invitations' table: %w", err)
	}

	_, err = DB.Exec(
		context.Background(),
		`
		CREATE TABLE IF NOT EXISTS organization_settings (
		    organization_id BIGINT NOT NULL
		        REFERENCES organizations(id) ON DELETE CASCADE,

		    visibility BOOLEAN NOT NULL DEFAULT FALSE, -- false = private, true = public
		    show_members BOOLEAN NOT NULL DEFAULT TRUE,

		    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		`,
	)

	if err != nil {
		return fmt.Errorf("store: create 'organization_settings' table: %w", err)
	}

	_, err = DB.Exec(
		context.Background(),
		`
		CREATE TABLE IF NOT EXISTS teams (
		    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

		    organization_id BIGINT NOT NULL
		        REFERENCES organizations(id) ON DELETE CASCADE,

		    name TEXT NOT NULL,
		    identifier TEXT NOT NULL,
		    description TEXT,

		    created_by BIGINT NOT NULL
		        REFERENCES users(id),

		    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

		    UNIQUE (organization_id, identifier)
		);
		`,
	)

	if err != nil {
		return fmt.Errorf("store: create 'teams' table: %w", err)
	}

	return nil
}
