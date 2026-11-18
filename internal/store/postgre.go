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

var DB *pgxpool.Pool

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

	return nil
}
