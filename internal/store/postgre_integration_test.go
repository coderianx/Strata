//go:build integration

// Database tests for the store package. They run only when a
// disposable PostgreSQL instance is configured:
//
//	TEST_DATABASE_URL=postgres://user:pass@localhost/strata_test \
//		go test -tags=integration ./...
//
// The tables are created from scratch, so point the URL at a test
// database, never a production one.

package store

import (
	"context"
	"fmt"
	"os"
	"testing"
)

const testDBEnv = "TEST_DATABASE_URL"

func TestMain(m *testing.M) {
	if dsn := os.Getenv(testDBEnv); dsn != "" {
		os.Setenv("DB_URL", dsn)

		if err := ConnectDatabase(); err != nil {
			fmt.Fprintf(os.Stderr, "integration: connect database: %v\n", err)
			os.Exit(1)
		}
	}

	code := m.Run()

	if DB != nil {
		DB.Close()
	}

	os.Exit(code)
}

// requireDB skips database tests when no test instance is configured.
func requireDB(t *testing.T) {
	t.Helper()

	if DB == nil {
		t.Skipf("%s is not set, skipping database test", testDBEnv)
	}
}

func TestCreateTables(t *testing.T) {
	requireDB(t)

	// The migrations must be idempotent: a database may already have
	// been migrated by a previous run.
	for i := 1; i <= 2; i++ {
		if err := CreateTables(); err != nil {
			t.Fatalf("CreateTables() run %d = %v, want nil", i, err)
		}
	}

	for _, table := range []string{"users", "organizations", "organization_members", "refresh_tokens"} {
		var exists bool

		err := DB.QueryRow(
			context.Background(),
			`SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_name = $1
			)`,
			table,
		).Scan(&exists)
		if err != nil {
			t.Fatalf("check %s table: %v", table, err)
		}

		if !exists {
			t.Errorf("CreateTables() did not create the %s table", table)
		}
	}
}
