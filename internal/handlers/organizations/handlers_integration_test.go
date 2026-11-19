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
	"github.com/coderianx/Strata/internal/models"
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
		`TRUNCATE users, organizations, organization_members, organization_invitations, organization_settings, teams, team_members, refresh_tokens RESTART IDENTITY CASCADE`,
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

func TestCreateNewOrganizationHandlerIntegration(t *testing.T) {
	requireDB(t)

	userID := insertUser(t, "org_owner", "owner@strata.dev")

	t.Run("creates the organization and an owner membership", func(t *testing.T) {
		// Padding and mixed case on purpose: the handler must
		// normalize before storing.
		body := `{"name":"  Coderian Studio ","slug":"  CODERIAN-STUDIO ","description":" Crafted software. ","logo_url":" https://strata.dev/logo.png ","visibility":true,"show_members":true}`

		rec := callAuthed(t, CreateNewOrganizationHandler, userID,
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

		var visibility, showMembers bool

		err = store.DB.QueryRow(
			context.Background(),
			`SELECT visibility, show_members FROM organization_settings
			 WHERE organization_id = (SELECT id FROM organizations WHERE slug = $1)`,
			"coderian-studio",
		).Scan(&visibility, &showMembers)
		if err != nil {
			t.Fatalf("load organization settings: %v", err)
		}

		if !visibility {
			t.Error("visibility = false, want true")
		}

		if !showMembers {
			t.Error("show_members = false, want true")
		}
	})

	t.Run("rejects a duplicate slug", func(t *testing.T) {
		body := `{"name":"Other","slug":"coderian-studio"}`

		rec := callAuthed(t, CreateNewOrganizationHandler, userID,
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
		false,
		false,
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

// insertOrg stores an organization owned by ownerID under a unique
// slug and returns its id.
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

// insertInvitation invites a user through the repository and returns
// the pending invitation's id.
func insertInvitation(t *testing.T, orgID, senderID int64, receiverUsername, role string) int64 {
	t.Helper()

	err := repository.InviteOrganization(
		context.Background(),
		orgID,
		senderID,
		receiverUsername,
		role,
	)
	if err != nil {
		t.Fatalf("insert fixture invitation: %v", err)
	}

	var invitationID int64

	err = store.DB.QueryRow(
		context.Background(),
		`SELECT id FROM organization_invitations
		 WHERE organization_id = $1 AND user_id = (SELECT id FROM users WHERE username = $2)`,
		orgID,
		receiverUsername,
	).Scan(&invitationID)
	if err != nil {
		t.Fatalf("load fixture invitation id: %v", err)
	}

	return invitationID
}

func TestListMyInvitationsHandlerIntegration(t *testing.T) {
	requireDB(t)

	ownerID := insertUser(t, "org_owner", "owner@strata.dev")
	receiverID := insertUser(t, "invitee", "invitee@strata.dev")
	insertUser(t, "other_user", "other@strata.dev")

	orgA := insertOrg(t, ownerID, "list-a")
	orgB := insertOrg(t, ownerID, "list-b")

	// The receiver has two pending invitations; the second one is
	// accepted below, so only the first stays listed.
	invitationA := insertInvitation(t, orgA, ownerID, "invitee", "member")
	invitationB := insertInvitation(t, orgB, ownerID, "invitee", "owner")

	// Another user's invitation must never show up in the receiver's
	// list.
	insertInvitation(t, orgA, ownerID, "other_user", "member")

	if err := repository.AcceptInvitation(context.Background(), receiverID, invitationB); err != nil {
		t.Fatalf("accept fixture invitation: %v", err)
	}

	rec := callAuthed(t, ListMyInvitationsHandler, receiverID, http.MethodGet, "/", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s",
			rec.Code, http.StatusOK, rec.Body.String())
	}

	var res struct {
		Invitations []models.ListReceivedInvitations `json:"invitations"`
	}
	decodeResponse(t, rec, &res)

	if len(res.Invitations) != 1 {
		t.Fatalf("invitations = %d, want 1: %+v", len(res.Invitations), res.Invitations)
	}

	inv := res.Invitations[0]

	if inv.ID != invitationA {
		t.Errorf("id = %d, want %d", inv.ID, invitationA)
	}

	if inv.OrganizationID != orgA {
		t.Errorf("organization_id = %d, want %d", inv.OrganizationID, orgA)
	}

	if inv.InvitedBy != ownerID {
		t.Errorf("invited_by = %d, want %d", inv.InvitedBy, ownerID)
	}

	if inv.Role != "member" {
		t.Errorf("role = %q, want %q", inv.Role, "member")
	}

	if inv.Status != "pending" {
		t.Errorf("status = %q, want %q", inv.Status, "pending")
	}

	if inv.CreatedAt.IsZero() {
		t.Error("created_at is zero, want a timestamp")
	}
}

func TestAcceptInvitationHandlerIntegration(t *testing.T) {
	requireDB(t)

	ownerID := insertUser(t, "org_owner", "owner@strata.dev")
	receiverID := insertUser(t, "invitee", "invitee@strata.dev")

	orgID := insertOrg(t, ownerID, "accept-org")
	invitationID := insertInvitation(t, orgID, ownerID, "invitee", "member")

	t.Run("accepts a pending invitation and grants membership", func(t *testing.T) {
		rec := callInvitation(t, "accept", receiverID, strconv.FormatInt(invitationID, 10))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusOK, rec.Body.String())
		}

		var res struct {
			Message string `json:"message"`
		}
		decodeResponse(t, rec, &res)

		if res.Message != "invitation_accepted" {
			t.Errorf("message = %q, want %q", res.Message, "invitation_accepted")
		}

		var status string

		err := store.DB.QueryRow(
			context.Background(),
			`SELECT status FROM organization_invitations WHERE id = $1`,
			invitationID,
		).Scan(&status)
		if err != nil {
			t.Fatalf("load invitation: %v", err)
		}

		if status != "accepted" {
			t.Errorf("invitation status = %q, want %q", status, "accepted")
		}

		var memberRole string

		err = store.DB.QueryRow(
			context.Background(),
			`SELECT role FROM organization_members
			 WHERE organization_id = $1 AND user_id = $2`,
			orgID,
			receiverID,
		).Scan(&memberRole)
		if err != nil {
			t.Fatalf("load membership: %v", err)
		}

		if memberRole != "member" {
			t.Errorf("membership role = %q, want %q", memberRole, "member")
		}
	})

	t.Run("cannot accept twice", func(t *testing.T) {
		rec := callInvitation(t, "accept", receiverID, strconv.FormatInt(invitationID, 10))

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusNotFound, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "invitation_not_found" {
			t.Errorf("error = %q, want %q", res.Error, "invitation_not_found")
		}
	})

	t.Run("cannot accept someone else's invitation", func(t *testing.T) {
		orgID2 := insertOrg(t, ownerID, "accept-org-2")
		otherInvitation := insertInvitation(t, orgID2, ownerID, "invitee", "member")

		rec := callInvitation(t, "accept", ownerID, strconv.FormatInt(otherInvitation, 10))

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusNotFound, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "invitation_not_found" {
			t.Errorf("error = %q, want %q", res.Error, "invitation_not_found")
		}
	})

	t.Run("unknown invitation id", func(t *testing.T) {
		rec := callInvitation(t, "accept", receiverID, "424242")

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "invitation_not_found" {
			t.Errorf("error = %q, want %q", res.Error, "invitation_not_found")
		}
	})

	t.Run("invalid invitation id", func(t *testing.T) {
		rec := callInvitation(t, "accept", receiverID, "not-a-number")

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "invalid_invitation_id" {
			t.Errorf("error = %q, want %q", res.Error, "invalid_invitation_id")
		}
	})
}

func TestRejectInvitationHandlerIntegration(t *testing.T) {
	requireDB(t)

	ownerID := insertUser(t, "org_owner", "owner@strata.dev")
	receiverID := insertUser(t, "invitee", "invitee@strata.dev")

	orgID := insertOrg(t, ownerID, "reject-org")
	invitationID := insertInvitation(t, orgID, ownerID, "invitee", "member")

	t.Run("rejects a pending invitation without granting membership", func(t *testing.T) {
		rec := callInvitation(t, "reject", receiverID, strconv.FormatInt(invitationID, 10))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusOK, rec.Body.String())
		}

		var res struct {
			Message string `json:"message"`
		}
		decodeResponse(t, rec, &res)

		if res.Message != "invitation_rejected" {
			t.Errorf("message = %q, want %q", res.Message, "invitation_rejected")
		}

		var status string

		err := store.DB.QueryRow(
			context.Background(),
			`SELECT status FROM organization_invitations WHERE id = $1`,
			invitationID,
		).Scan(&status)
		if err != nil {
			t.Fatalf("load invitation: %v", err)
		}

		if status != "rejected" {
			t.Errorf("invitation status = %q, want %q", status, "rejected")
		}

		var memberCount int

		err = store.DB.QueryRow(
			context.Background(),
			`SELECT COUNT(*) FROM organization_members
			 WHERE organization_id = $1 AND user_id = $2`,
			orgID,
			receiverID,
		).Scan(&memberCount)
		if err != nil {
			t.Fatalf("count memberships: %v", err)
		}

		if memberCount != 0 {
			t.Errorf("memberships = %d, want 0", memberCount)
		}
	})

	t.Run("a rejected user can be invited again", func(t *testing.T) {
		// The pending-only unique index allows a fresh invitation
		// after a rejection.
		err := repository.InviteOrganization(
			context.Background(),
			orgID,
			ownerID,
			"invitee",
			"member",
		)
		if err != nil {
			t.Errorf("re-invite after rejection = %v, want nil", err)
		}
	})

	t.Run("cannot reject someone else's invitation", func(t *testing.T) {
		orgID2 := insertOrg(t, ownerID, "reject-org-2")
		otherInvitation := insertInvitation(t, orgID2, ownerID, "invitee", "member")

		rec := callInvitation(t, "reject", ownerID, strconv.FormatInt(otherInvitation, 10))

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusNotFound, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "invitation_not_found" {
			t.Errorf("error = %q, want %q", res.Error, "invitation_not_found")
		}
	})

	t.Run("unknown invitation id", func(t *testing.T) {
		rec := callInvitation(t, "reject", receiverID, "424242")

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "invitation_not_found" {
			t.Errorf("error = %q, want %q", res.Error, "invitation_not_found")
		}
	})

	t.Run("invalid invitation id", func(t *testing.T) {
		rec := callInvitation(t, "reject", receiverID, "not-a-number")

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "invalid_invitation_id" {
			t.Errorf("error = %q, want %q", res.Error, "invalid_invitation_id")
		}
	})
}

// insertOrgWithSettings stores an organization with explicit
// visibility and show_members values and returns its id.
func insertOrgWithSettings(
	t *testing.T,
	ownerID int64,
	slug string,
	visibility, showMembers bool,
) int64 {
	t.Helper()

	err := repository.CreateNewOrganization(
		context.Background(),
		"Strata "+slug,
		slug,
		"organization test fixture",
		"",
		ownerID,
		visibility,
		showMembers,
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

// acceptInvitationFor adds an existing user to an organization by
// inviting and accepting through the repository.
func acceptInvitationFor(t *testing.T, orgID, senderID, userID int64, username string) {
	t.Helper()

	invitationID := insertInvitation(t, orgID, senderID, username, "member")

	if err := repository.AcceptInvitation(context.Background(), userID, invitationID); err != nil {
		t.Fatalf("accept fixture invitation: %v", err)
	}
}

func TestGetMyOrganizationsHandlerIntegration(t *testing.T) {
	requireDB(t)

	ownerID := insertUser(t, "org_owner", "owner@strata.dev")
	otherID := insertUser(t, "other_owner", "other@strata.dev")

	insertOrg(t, ownerID, "my-org-1")
	insertOrg(t, ownerID, "my-org-2")

	// Another user's organization must never show up in the list.
	insertOrg(t, otherID, "other-org")

	rec := callAuthed(t, GetMyOrganizationsHandler, ownerID, http.MethodGet, "/", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s",
			rec.Code, http.StatusOK, rec.Body.String())
	}

	var res struct {
		Organizations []models.MyOrganizations `json:"organizations"`
	}
	decodeResponse(t, rec, &res)

	if len(res.Organizations) != 2 {
		t.Fatalf("organizations = %d, want 2: %+v", len(res.Organizations), res.Organizations)
	}

	for _, org := range res.Organizations {
		if org.Slug != "my-org-1" && org.Slug != "my-org-2" {
			t.Errorf("unexpected organization slug %q", org.Slug)
		}

		if org.Name == "" || org.ID <= 0 || org.CreatedAt.IsZero() {
			t.Errorf("organization %q is missing fields: %+v", org.Slug, org)
		}
	}
}

func TestOrganizationMembersHandlerIntegration(t *testing.T) {
	requireDB(t)

	ownerID := insertUser(t, "org_owner", "owner@strata.dev")
	memberID := insertUser(t, "org_member", "member@strata.dev")
	outsiderID := insertUser(t, "outsider", "outsider@strata.dev")

	// Private with a hidden member list: only members may see it.
	orgID := insertOrgWithSettings(t, ownerID, "members-org", false, false)

	acceptInvitationFor(t, orgID, ownerID, memberID, "org_member")

	publicHidden := insertOrgWithSettings(t, ownerID, "public-hidden", true, false)
	publicShown := insertOrgWithSettings(t, ownerID, "public-shown", true, true)

	t.Run("members always see the list", func(t *testing.T) {
		// The organization is private with show_members disabled and
		// the caller is a member: the settings must not matter.
		rec := callMembers(t, memberID, strconv.FormatInt(orgID, 10))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusOK, rec.Body.String())
		}

		var res struct {
			Members []repository.OrganizationMember `json:"members"`
		}
		decodeResponse(t, rec, &res)

		if len(res.Members) != 2 {
			t.Fatalf("members = %d, want 2: %+v", len(res.Members), res.Members)
		}

		roles := map[string]string{}

		for _, member := range res.Members {
			if member.ID <= 0 || member.Name == "" || member.JoinedAt.IsZero() {
				t.Errorf("member %q is missing fields: %+v", member.Username, member)
			}

			roles[member.Username] = member.Role
		}

		if roles["org_owner"] != "owner" {
			t.Errorf("org_owner role = %q, want %q", roles["org_owner"], "owner")
		}

		if roles["org_member"] != "member" {
			t.Errorf("org_member role = %q, want %q", roles["org_member"], "member")
		}
	})

	t.Run("a non-member gets no hint that a private organization exists", func(t *testing.T) {
		rec := callMembers(t, outsiderID, strconv.FormatInt(orgID, 10))

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

	t.Run("public organization with hidden members", func(t *testing.T) {
		rec := callMembers(t, outsiderID, strconv.FormatInt(publicHidden, 10))

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusForbidden, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "members_confidential" {
			t.Errorf("error = %q, want %q", res.Error, "members_confidential")
		}
	})

	t.Run("public organization with visible members", func(t *testing.T) {
		rec := callMembers(t, outsiderID, strconv.FormatInt(publicShown, 10))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusOK, rec.Body.String())
		}

		var res struct {
			Members []repository.OrganizationMember `json:"members"`
		}
		decodeResponse(t, rec, &res)

		if len(res.Members) != 1 {
			t.Errorf("members = %d, want 1 (the owner only)", len(res.Members))
		}
	})

	t.Run("unknown organization", func(t *testing.T) {
		rec := callMembers(t, ownerID, "424242")

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
		rec := callMembers(t, ownerID, "not-a-number")

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

func TestRemoveMemberHandlerIntegration(t *testing.T) {
	requireDB(t)

	ownerID := insertUser(t, "org_owner", "owner@strata.dev")
	memberID := insertUser(t, "org_member", "member@strata.dev")
	anotherID := insertUser(t, "another_user", "another@strata.dev")

	orgID := insertOrg(t, ownerID, "remove-org")
	orgIDParam := strconv.FormatInt(orgID, 10)

	acceptInvitationFor(t, orgID, ownerID, memberID, "org_member")

	t.Run("the owner removes a member", func(t *testing.T) {
		rec := callRemove(t, ownerID, orgIDParam, strconv.FormatInt(memberID, 10))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusOK, rec.Body.String())
		}

		var res struct {
			Message string `json:"message"`
		}
		decodeResponse(t, rec, &res)

		if res.Message != "member_removed" {
			t.Errorf("message = %q, want %q", res.Message, "member_removed")
		}

		var count int

		err := store.DB.QueryRow(
			context.Background(),
			`SELECT COUNT(*) FROM organization_members
			 WHERE organization_id = $1 AND user_id = $2`,
			orgID,
			memberID,
		).Scan(&count)
		if err != nil {
			t.Fatalf("count memberships: %v", err)
		}

		if count != 0 {
			t.Errorf("memberships left = %d, want 0", count)
		}
	})

	t.Run("the owner cannot remove themselves", func(t *testing.T) {
		rec := callRemove(t, ownerID, orgIDParam, strconv.FormatInt(ownerID, 10))

		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusConflict, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "cannot_remove_owner" {
			t.Errorf("error = %q, want %q", res.Error, "cannot_remove_owner")
		}
	})

	t.Run("a non-owner member cannot remove anyone", func(t *testing.T) {
		// Restore the membership removed by the first subtest.
		acceptInvitationFor(t, orgID, ownerID, memberID, "org_member")

		rec := callRemove(t, memberID, orgIDParam, strconv.FormatInt(anotherID, 10))

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

	t.Run("removing a non-member", func(t *testing.T) {
		rec := callRemove(t, ownerID, orgIDParam, strconv.FormatInt(anotherID, 10))

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusNotFound, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "member_not_found" {
			t.Errorf("error = %q, want %q", res.Error, "member_not_found")
		}
	})

	t.Run("unknown organization", func(t *testing.T) {
		rec := callRemove(t, ownerID, "424242", strconv.FormatInt(memberID, 10))

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

	t.Run("invalid member id", func(t *testing.T) {
		rec := callRemove(t, ownerID, orgIDParam, "not-a-number")

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "invalid_member_id" {
			t.Errorf("error = %q, want %q", res.Error, "invalid_member_id")
		}
	})

	t.Run("invalid organization id", func(t *testing.T) {
		rec := callRemove(t, ownerID, "not-a-number", strconv.FormatInt(memberID, 10))

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
