//go:build integration

// End-to-end tests for the team handlers backed by a real PostgreSQL
// instance. Run them with:
//
//	TEST_DATABASE_URL=postgres://user:pass@localhost/strata_test \
//		go test -tags=integration ./...
//
// The database must be disposable: every test truncates all tables.

package teams

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"testing"

	"github.com/coderianx/Strata/internal/repository"
	"github.com/coderianx/Strata/internal/store"
	"golang.org/x/crypto/bcrypt"
)

const testDBEnv = "TEST_DATABASE_URL"

func TestMain(m *testing.M) {
	if dsn := os.Getenv(testDBEnv); dsn != "" {
		os.Setenv("DB_URL", dsn)
		os.Setenv("JWT_SECRET", testSecret)

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

// requireDB skips the test when no test database is configured and
// resets the tables so every test starts from a clean state.
func requireDB(t *testing.T) {
	t.Helper()

	if store.DB == nil {
		t.Skipf("%s is not set, skipping database test", testDBEnv)
	}

	_, err := store.DB.Exec(
		context.Background(),
		`TRUNCATE users, organizations, organization_members, organization_invitations, organization_settings, teams, refresh_tokens RESTART IDENTITY CASCADE`,
	)
	if err != nil {
		t.Fatalf("reset tables: %v", err)
	}
}

// insertUser stores a fixture user directly through the repository.
// The bcrypt hash uses the minimum cost: fixture strength is
// irrelevant, test speed is not.
func insertUser(t *testing.T, username, email string) int64 {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte("sifre1234"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash fixture password: %v", err)
	}

	id, err := repository.InsertNewUser(
		context.Background(),
		"Test User",
		username,
		email,
		string(hash),
	)
	if err != nil {
		t.Fatalf("insert fixture user: %v", err)
	}

	return id
}

// insertOrg stores a private organization owned by ownerID and
// returns its id.
func insertOrg(t *testing.T, ownerID int64, slug string) int64 {
	t.Helper()

	err := repository.CreateNewOrganization(
		context.Background(),
		"Strata "+slug,
		slug,
		"organization test fixture",
		"",
		ownerID,
		false,
		false,
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

// insertTeam stores a team through the repository and returns its
// id.
func insertTeam(t *testing.T, orgID, ownerID int64, identifier string) int64 {
	t.Helper()

	teamID, err := repository.CreateNewTeam(
		context.Background(),
		orgID,
		"Team "+identifier,
		identifier,
		"team test fixture",
		ownerID,
	)
	if err != nil {
		t.Fatalf("insert fixture team: %v", err)
	}

	return teamID
}

func TestCreateNewTeamHandlerIntegration(t *testing.T) {
	requireDB(t)

	ownerID := insertUser(t, "org_owner", "owner@strata.dev")
	outsiderID := insertUser(t, "outsider", "outsider@strata.dev")

	orgID := insertOrg(t, ownerID, "teams-org")
	orgIDParam := strconv.FormatInt(orgID, 10)

	t.Run("creates a team for the owner", func(t *testing.T) {
		// Padding and mixed case on purpose: the handler must
		// normalize before storing.
		body := `{"name":"  Backend Team ","identifier":"  BACKEND-TEAM ","description":" Crafted software. "}`

		rec := callCreateTeam(t, ownerID, orgIDParam, body)

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusCreated, rec.Body.String())
		}

		var res struct {
			Message string `json:"message"`
			TeamID  int64  `json:"teamID"`
		}
		decodeResponse(t, rec, &res)

		if res.Message != "team_created" {
			t.Errorf("message = %q, want %q", res.Message, "team_created")
		}

		if res.TeamID <= 0 {
			t.Errorf("teamID = %d, want a positive id", res.TeamID)
		}

		var name, identifier, description string
		var createdBy int64

		err := store.DB.QueryRow(
			context.Background(),
			`SELECT name, identifier, description, created_by
			 FROM teams WHERE organization_id = $1 AND identifier = $2`,
			orgID,
			"backend-team",
		).Scan(&name, &identifier, &description, &createdBy)
		if err != nil {
			t.Fatalf("load stored team: %v", err)
		}

		if name != "Backend Team" {
			t.Errorf("name = %q, want %q", name, "Backend Team")
		}

		if identifier != "backend-team" {
			t.Errorf("identifier = %q, want %q", identifier, "backend-team")
		}

		if description != "Crafted software." {
			t.Errorf("description = %q, want %q", description, "Crafted software.")
		}

		if createdBy != ownerID {
			t.Errorf("created_by = %d, want %d", createdBy, ownerID)
		}
	})

	t.Run("rejects a duplicate identifier", func(t *testing.T) {
		body := `{"name":"Other Team","identifier":"backend-team"}`

		rec := callCreateTeam(t, ownerID, orgIDParam, body)

		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusConflict, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "team_already_exists" {
			t.Errorf("error = %q, want %q", res.Error, "team_already_exists")
		}
	})

	t.Run("only the owner can create teams", func(t *testing.T) {
		rec := callCreateTeam(t, outsiderID, orgIDParam,
			`{"name":"Outsider Team","identifier":"outsider-team"}`)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusNotFound, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "organization_not_found_or_access_denied" {
			t.Errorf("error = %q, want %q",
				res.Error, "organization_not_found_or_access_denied")
		}
	})

	t.Run("unknown organization", func(t *testing.T) {
		rec := callCreateTeam(t, ownerID, "424242",
			`{"name":"Ghost Team","identifier":"ghost-team"}`)

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "organization_not_found_or_access_denied" {
			t.Errorf("error = %q, want %q",
				res.Error, "organization_not_found_or_access_denied")
		}
	})

	t.Run("invalid organization id", func(t *testing.T) {
		rec := callCreateTeam(t, ownerID, "not-a-number", "")

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "invalid_organization_id" {
			t.Errorf("error = %q, want %q", res.Error, "invalid_organization_id")
		}
	})
}

func TestDeleteTeamHandlerIntegration(t *testing.T) {
	requireDB(t)

	ownerID := insertUser(t, "org_owner", "owner@strata.dev")
	memberID := insertUser(t, "org_member", "member@strata.dev")

	orgA := insertOrg(t, ownerID, "teams-org-a")
	orgB := insertOrg(t, ownerID, "teams-org-b")

	teamA := insertTeam(t, orgA, ownerID, "delete-me")
	teamB := insertTeam(t, orgB, ownerID, "keep-me")

	orgAParam := strconv.FormatInt(orgA, 10)
	teamAParam := strconv.FormatInt(teamA, 10)
	teamBParam := strconv.FormatInt(teamB, 10)

	t.Run("the owner deletes their team", func(t *testing.T) {
		rec := callDeleteTeam(t, ownerID, orgAParam, teamAParam)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusOK, rec.Body.String())
		}

		var res struct {
			Message string `json:"message"`
		}
		decodeResponse(t, rec, &res)

		if res.Message != "team_deleted" {
			t.Errorf("message = %q, want %q", res.Message, "team_deleted")
		}

		var count int

		err := store.DB.QueryRow(
			context.Background(),
			`SELECT COUNT(*) FROM teams WHERE id = $1`,
			teamA,
		).Scan(&count)
		if err != nil {
			t.Fatalf("count teams: %v", err)
		}

		if count != 0 {
			t.Errorf("teams left = %d, want 0", count)
		}
	})

	t.Run("teams of other organizations are not reachable", func(t *testing.T) {
		rec := callDeleteTeam(t, ownerID, orgAParam, teamBParam)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusNotFound, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "team_not_found_or_access_denied" {
			t.Errorf("error = %q, want %q",
				res.Error, "team_not_found_or_access_denied")
		}
	})

	t.Run("a non-owner cannot delete", func(t *testing.T) {
		rec := callDeleteTeam(t, memberID, orgAParam, teamBParam)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusNotFound, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "organization_not_found_or_access_denied" {
			t.Errorf("error = %q, want %q",
				res.Error, "organization_not_found_or_access_denied")
		}
	})

	t.Run("unknown team id", func(t *testing.T) {
		rec := callDeleteTeam(t, ownerID, orgAParam, "424242")

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "team_not_found_or_access_denied" {
			t.Errorf("error = %q, want %q",
				res.Error, "team_not_found_or_access_denied")
		}
	})

	t.Run("invalid team id", func(t *testing.T) {
		rec := callDeleteTeam(t, ownerID, orgAParam, "not-a-number")

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "invalid_team_id" {
			t.Errorf("error = %q, want %q", res.Error, "invalid_team_id")
		}
	})

	t.Run("invalid organization id", func(t *testing.T) {
		rec := callDeleteTeam(t, ownerID, "not-a-number", teamAParam)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "invalid_organization_id" {
			t.Errorf("error = %q, want %q", res.Error, "invalid_organization_id")
		}
	})
}
