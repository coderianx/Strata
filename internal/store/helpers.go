package store

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// uniqueViolation is the PostgreSQL SQLSTATE code for a unique
// constraint violation.
const uniqueViolation = "23505"

// IsUniqueErr reports whether err is, or wraps, a PostgreSQL unique
// constraint violation. Callers use it to turn duplicate-key failures
// (e.g. an already taken username, email or slug) into user-facing
// errors instead of treating them as internal failures.
func IsUniqueErr(err error) bool {
	var pgErr *pgconn.PgError

	if errors.As(err, &pgErr) {
		return pgErr.Code == uniqueViolation
	}

	return false
}
