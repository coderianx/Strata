package store

import (
	"strings"
	"testing"
)

// These tests exercise only the failure paths of ConnectDatabase,
// which never leave a live pool in the global DB.

func TestConnectDatabaseMissingURL(t *testing.T) {
	t.Setenv("DB_URL", "")

	err := ConnectDatabase()

	if err == nil {
		t.Fatal("ConnectDatabase() = nil, want error when DB_URL is unset")
	}

	if !strings.Contains(err.Error(), "DB_URL is not set") {
		t.Errorf("ConnectDatabase() = %q, want it to mention DB_URL", err)
	}
}

func TestConnectDatabaseInvalidURL(t *testing.T) {
	// "port" is not a port number, so parsing fails before any dial.
	t.Setenv("DB_URL", "postgres://user:pass@localhost:port/strata")

	err := ConnectDatabase()

	if err == nil {
		t.Fatal("ConnectDatabase() = nil, want error for an invalid DSN")
	}

	if DB != nil {
		t.Error("DB must be nil after a failed connect")
	}
}

func TestConnectDatabaseUnreachable(t *testing.T) {
	// Nothing listens on 127.0.0.1:1; the pool must fail its ping,
	// close itself and clear the global. connect_timeout keeps the
	// dial bounded on hosts that drop packets instead of refusing.
	t.Setenv(
		"DB_URL",
		"postgres://strata:strata@127.0.0.1:1/strata?sslmode=disable&connect_timeout=1",
	)

	err := ConnectDatabase()

	if err == nil {
		t.Fatal("ConnectDatabase() = nil, want error when the server is unreachable")
	}

	if DB != nil {
		t.Error("DB must be closed and nil after a failed ping")
	}
}
