//go:build integration

// End-to-end tests for the organization handlers backed by a real
// PostgreSQL instance. Run them with:
//
//	TEST_DATABASE_URL=postgres://user:pass@localhost/strata_test \
//		go test -tags=integration ./...
//
// The database must be disposable: every test truncates all tables.

package organizations

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/coderianx/Strata/internal/middlewares"
	"github.com/coderianx/Strata/internal/repository"
	"github.com/coderianx/Strata/internal/store"
	"github.com/go-chi/chi/v5"
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
		`TRUNCATE users, organizations, organization_members, refresh_tokens RESTART IDENTITY CASCADE`,
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

// deleteRouter mounts DeleteOrganization behind the middleware under
// the production-style route pattern, so chi.URLParam is populated
// exactly like in the real router.
func deleteRouter() http.Handler {
	router := chi.NewRouter()

	router.Route("/api/v1/organizations", func(r chi.Router) {
		r.With(middlewares.Auth).Delete("/{organization_id}", DeleteOrganization)
	})

	return router
}

func TestCreateNewOrganizationIntegration(t *testing.T) {
	requireDB(t)

	userID := insertUser(t, "org_owner", "owner@strata.dev")

	t.Run("creates the organization and an owner membership", func(t *testing.T) {
		// Padding and mixed case on purpose: the handler must
		// normalize before storing.
		body := `{"name":"  Coderian Studio ","slug":"  CODERIAN-STUDIO ","description":" Crafted software. ","logo_url":" https://strata.dev/logo.png "}`

		rec := callAuthed(t, CreateNewOrganization, userID,
			http.MethodPost, "/api/v1/organizations", body)

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusCreated, rec.Body.String())
		}

		var res struct {
			Message string `json:"message"`
		}
		decodeResponse(t, rec, &res)

		if res.Message != "organization_created" {
			t.Errorf("message = %q, want %q", res.Message, "organization_created")
		}

		// The stored row must hold the normalized values and the
		// caller as its owner.
		var name, slug, description, logoURL string
		var ownerID int64

		err := store.DB.QueryRow(
			context.Background(),
			`SELECT name, slug, description, logo_url, owner_id
			 FROM organizations WHERE slug = $1`,
			"coderian-studio",
		).Scan(&name, &slug, &description, &logoURL, &ownerID)
		if err != nil {
			t.Fatalf("load stored organization: %v", err)
		}

		if name != "Coderian Studio" {
			t.Errorf("name = %q, want %q", name, "Coderian Studio")
		}

		if slug != "coderian-studio" {
			t.Errorf("slug = %q, want %q", slug, "coderian-studio")
		}

		if description != "Crafted software." {
			t.Errorf("description = %q, want %q", description, "Crafted software.")
		}

		if logoURL != "https://strata.dev/logo.png" {
			t.Errorf("logo_url = %q, want %q", logoURL, "https://strata.dev/logo.png")
		}

		if ownerID != userID {
			t.Errorf("owner_id = %d, want %d", ownerID, userID)
		}

		var role string

		err = store.DB.QueryRow(
			context.Background(),
			`SELECT role FROM organization_members
			 WHERE organization_id = (SELECT id FROM organizations WHERE slug = $1)
			   AND user_id = $2`,
			"coderian-studio",
			userID,
		).Scan(&role)
		if err != nil {
			t.Fatalf("load owner membership: %v", err)
		}

		if role != "owner" {
			t.Errorf("owner role = %q, want %q", role, "owner")
		}
	})

	t.Run("rejects a duplicate slug", func(t *testing.T) {
		body := `{"name":"Other","slug":"coderian-studio"}`

		rec := callAuthed(t, CreateNewOrganization, userID,
			http.MethodPost, "/api/v1/organizations", body)

		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusConflict, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "organization_already_exists" {
			t.Errorf("error = %q, want %q", res.Error, "organization_already_exists")
		}
	})
}

func TestDeleteOrganizationIntegration(t *testing.T) {
	requireDB(t)

	ownerID := insertUser(t, "org_owner", "owner@strata.dev")
	router := deleteRouter()

	// callDelete sends an authenticated DELETE request for the given
	// organization id through the chi router.
	callDelete := func(userID int64, organizationID string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(
			http.MethodDelete,
			"/api/v1/organizations/"+organizationID,
			nil,
		)
		req.Header.Set("Authorization", "Bearer "+makeToken(t, testSecret, userID))

		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		return rec
	}

	// insertOrg stores an organization owned by ownerID and returns
	// its id.
	insertOrg := func(slug string) int64 {
		t.Helper()

		err := repository.CreateNewOrganization(
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

	t.Run("deletes the caller's organization", func(t *testing.T) {
		orgID := insertOrg("delete-me")

		rec := callDelete(ownerID, strconv.FormatInt(orgID, 10))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusOK, rec.Body.String())
		}

		var res struct {
			Message string `json:"message"`
		}
		decodeResponse(t, rec, &res)

		if res.Message != "organization_deleted" {
			t.Errorf("message = %q, want %q", res.Message, "organization_deleted")
		}

		var count int

		err := store.DB.QueryRow(
			context.Background(),
			`SELECT COUNT(*) FROM organizations WHERE id = $1`,
			orgID,
		).Scan(&count)
		if err != nil {
			t.Fatalf("count organizations: %v", err)
		}

		if count != 0 {
			t.Errorf("organizations left = %d, want 0", count)
		}
	})

	t.Run("does not reveal other users' organizations", func(t *testing.T) {
		orgID := insertOrg("keep-me")

		notOwnerID := insertUser(t, "not_owner", "notowner@strata.dev")

		rec := callDelete(notOwnerID, strconv.FormatInt(orgID, 10))

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusNotFound, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "organization_not_found" {
			t.Errorf("error = %q, want %q", res.Error, "organization_not_found")
		}

		var count int

		err := store.DB.QueryRow(
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
		rec := callDelete(ownerID, "424242")

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "organization_not_found" {
			t.Errorf("error = %q, want %q", res.Error, "organization_not_found")
		}
	})

	t.Run("invalid id", func(t *testing.T) {
		rec := callDelete(ownerID, "not-a-number")

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

func TestInviteUserToOrganizationHandlerIntegration(t *testing.T) {
	requireDB(t)

	ownerID := insertUser(t, "org_owner", "owner@strata.dev")
	receiverID := insertUser(t, "invitee", "invitee@strata.dev")

	if err := repository.CreateNewOrganization(
		context.Background(),
		"Strata",
		"strata",
		"organization test fixture",
		"",
		ownerID,
	); err != nil {
		t.Fatalf("insert fixture organization: %v", err)
	}

	var orgID int64

	err := store.DB.QueryRow(
		context.Background(),
		`SELECT id FROM organizations WHERE slug = $1`,
		"strata",
	).Scan(&orgID)
	if err != nil {
		t.Fatalf("load fixture organization id: %v", err)
	}

	orgIDParam := strconv.FormatInt(orgID, 10)

	t.Run("invites a user as a member", func(t *testing.T) {
		rec := callInvite(t, ownerID, orgIDParam, `{"username":"invitee","role":"member"}`)

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusCreated, rec.Body.String())
		}

		var res struct {
			Message string `json:"message"`
		}
		decodeResponse(t, rec, &res)

		if res.Message != "user_invited" {
			t.Errorf("message = %q, want %q", res.Message, "user_invited")
		}

		var invitedBy int64
		var role, status string

		err := store.DB.QueryRow(
			context.Background(),
			`SELECT invited_by, role, status FROM organization_invitations
			 WHERE organization_id = $1 AND user_id = $2`,
			orgID,
			receiverID,
		).Scan(&invitedBy, &role, &status)
		if err != nil {
			t.Fatalf("load invitation row: %v", err)
		}

		if invitedBy != ownerID {
			t.Errorf("invited_by = %d, want %d", invitedBy, ownerID)
		}

		if role != "member" {
			t.Errorf("role = %q, want %q", role, "member")
		}

		if status != "pending" {
			t.Errorf("status = %q, want %q", status, "pending")
		}
	})

	t.Run("a pending invitation cannot be duplicated", func(t *testing.T) {
		rec := callInvite(t, ownerID, orgIDParam, `{"username":"invitee","role":"owner"}`)

		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusConflict, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "invitation_already_exists" {
			t.Errorf("error = %q, want %q", res.Error, "invitation_already_exists")
		}
	})

	t.Run("a member cannot be invited twice", func(t *testing.T) {
		// The owner is already a member from the organization's
		// creation.
		rec := callInvite(t, ownerID, orgIDParam, `{"username":"org_owner","role":"member"}`)

		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusConflict, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "user_already_member" {
			t.Errorf("error = %q, want %q", res.Error, "user_already_member")
		}
	})

	t.Run("unknown receiver", func(t *testing.T) {
		rec := callInvite(t, ownerID, orgIDParam, `{"username":"ghost_user","role":"member"}`)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusNotFound, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "user_not_found" {
			t.Errorf("error = %q, want %q", res.Error, "user_not_found")
		}
	})

	t.Run("only the owner can invite", func(t *testing.T) {
		rec := callInvite(t, receiverID, orgIDParam, `{"username":"invitee","role":"member"}`)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusNotFound, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "organization_not_found" {
			t.Errorf("error = %q, want %q", res.Error, "organization_not_found")
		}
	})

	t.Run("unknown organization", func(t *testing.T) {
		rec := callInvite(t, ownerID, "424242", `{"username":"invitee","role":"member"}`)

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "organization_not_found" {
			t.Errorf("error = %q, want %q", res.Error, "organization_not_found")
		}
	})

	t.Run("invalid organization id", func(t *testing.T) {
		rec := callInvite(t, ownerID, "not-a-number", `{"username":"invitee","role":"member"}`)

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
