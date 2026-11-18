//go:build integration

// Database tests for the organizations repository. Like the auth
// repository tests, they run only when a disposable PostgreSQL instance
// is configured:
//
//	TEST_DATABASE_URL=postgres://user:pass@localhost/strata_test \
//		go test -tags=integration ./...
//
// requireDB truncates every table, so point the URL at a test
// database, never a production one.

package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/coderianx/Strata/internal/store"
)

func TestCreateNewOrganization(t *testing.T) {
	requireDB(t)

	ownerID := insertUser(t, "org_owner", "owner@strata.dev")

	t.Run("stores the organization and its owner membership", func(t *testing.T) {
		err := CreateNewOrganization(
			context.Background(),
			"Strata",
			"strata",
			"organization test fixture",
			"https://strata.dev/logo.png",
			ownerID,
		)
		if err != nil {
			t.Fatalf("CreateNewOrganization() = %v, want nil", err)
		}

		var orgID int64

		err = store.DB.QueryRow(
			context.Background(),
			`SELECT id FROM organizations WHERE slug = $1`,
			"strata",
		).Scan(&orgID)
		if err != nil {
			t.Fatalf("load fixture organization: %v", err)
		}

		var role string

		err = store.DB.QueryRow(
			context.Background(),
			`SELECT role FROM organization_members WHERE organization_id = $1 AND user_id = $2`,
			orgID,
			ownerID,
		).Scan(&role)
		if err != nil {
			t.Fatalf("load owner membership row: %v", err)
		}

		if role != "owner" {
			t.Errorf("owner role = %q, want %q", role, "owner")
		}
	})

	t.Run("rejects a duplicate slug", func(t *testing.T) {
		err := CreateNewOrganization(
			context.Background(),
			"Strata Two",
			"strata",
			"another fixture",
			"",
			ownerID,
		)
		if !errors.Is(err, ErrOrganizationAlreadyExists) {
			t.Errorf("CreateNewOrganization() = %v, want ErrOrganizationAlreadyExists", err)
		}

		var count int

		err = store.DB.QueryRow(
			context.Background(),
			`SELECT COUNT(*) FROM organizations WHERE slug = $1`,
			"strata",
		).Scan(&count)
		if err != nil {
			t.Fatalf("count fixture organizations: %v", err)
		}

		if count != 1 {
			t.Errorf("organizations with slug 'strata' = %d, want 1", count)
		}
	})
}

func TestDeleteOrganization(t *testing.T) {
	requireDB(t)

	ownerID := insertUser(t, "org_owner", "owner@strata.dev")

	// insertOrg stores an organization owned by ownerID under a unique
	// slug and returns its id, so each subtest works on its own rows.
	insertOrg := func(t *testing.T, slug string) int64 {
		t.Helper()

		err := CreateNewOrganization(
			context.Background(),
			"Strata "+slug,
			slug,
			"organization test fixture",
			"",
			ownerID,
		)
		if err != nil {
			t.Fatalf("insert fixture organization: %v", err)
		}

		var orgID int64

		err = store.DB.QueryRow(
			context.Background(),
			`SELECT id FROM organizations WHERE slug = $1`,
			slug,
		).Scan(&orgID)
		if err != nil {
			t.Fatalf("load fixture organization id: %v", err)
		}

		return orgID
	}

	t.Run("removes the organization and its members", func(t *testing.T) {
		orgID := insertOrg(t, "delete-me")

		err := DeleteOrganization(context.Background(), orgID, ownerID)
		if err != nil {
			t.Fatalf("DeleteOrganization() = %v, want nil", err)
		}

		var orgCount int

		err = store.DB.QueryRow(
			context.Background(),
			`SELECT COUNT(*) FROM organizations WHERE id = $1`,
			orgID,
		).Scan(&orgCount)
		if err != nil {
			t.Fatalf("count organizations: %v", err)
		}

		if orgCount != 0 {
			t.Errorf("organizations left = %d, want 0", orgCount)
		}

		var memberCount int

		err = store.DB.QueryRow(
			context.Background(),
			`SELECT COUNT(*) FROM organization_members WHERE organization_id = $1`,
			orgID,
		).Scan(&memberCount)
		if err != nil {
			t.Fatalf("count organization members: %v", err)
		}

		if memberCount != 0 {
			t.Errorf("organization members left = %d, want 0 (ON DELETE CASCADE)", memberCount)
		}
	})

	t.Run("does not delete when the caller is not the owner", func(t *testing.T) {
		orgID := insertOrg(t, "keep-me")

		notOwnerID := insertUser(t, "not_owner", "notowner@strata.dev")

		err := DeleteOrganization(context.Background(), orgID, notOwnerID)
		if !errors.Is(err, ErrOrganizationNotFound) {
			t.Errorf("DeleteOrganization() = %v, want ErrOrganizationNotFound", err)
		}

		var count int

		err = store.DB.QueryRow(
			context.Background(),
			`SELECT COUNT(*) FROM organizations WHERE id = $1`,
			orgID,
		).Scan(&count)
		if err != nil {
			t.Fatalf("count organizations: %v", err)
		}

		if count != 1 {
			t.Errorf("organizations left = %d, want 1", count)
		}
	})

	t.Run("unknown id", func(t *testing.T) {
		err := DeleteOrganization(context.Background(), 424242, ownerID)
		if !errors.Is(err, ErrOrganizationNotFound) {
			t.Errorf("DeleteOrganization() = %v, want ErrOrganizationNotFound", err)
		}
	})
}
