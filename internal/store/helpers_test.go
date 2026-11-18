package store

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsUniqueErr(t *testing.T) {
	uniqueErr := &pgconn.PgError{
		Code:           "23505",
		ConstraintName: "users_email_key",
		Message:        `duplicate key value violates unique constraint "users_email_key"`,
	}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "plain error",
			err:  errors.New("store: something else went wrong"),
			want: false,
		},
		{
			name: "unique violation",
			err:  uniqueErr,
			want: true,
		},
		{
			name: "wrapped unique violation",
			err:  fmt.Errorf("store: insert user: %w", uniqueErr),
			want: true,
		},
		{
			name: "other SQLSTATE code",
			err: &pgconn.PgError{
				Code:    "23503",
				Message: `insert or update on table "refresh_tokens" violates foreign key constraint`,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUniqueErr(tt.err); got != tt.want {
				t.Errorf("IsUniqueErr(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
