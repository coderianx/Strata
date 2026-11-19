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

	"github.com/coderianx/Strata/internal/models"
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

		// The creator must be written as a team member with the
		// 'creator' role, in the same transaction as the team.
		var memberRole string

		err = store.DB.QueryRow(
			context.Background(),
			`SELECT role FROM team_members
			 WHERE team_id = (SELECT id FROM teams WHERE organization_id = $1 AND identifier = $2)
			   AND user_id = $3`,
			orgID,
			"backend-team",
			ownerID,
		).Scan(&memberRole)
		if err != nil {
			t.Fatalf("load creator membership: %v", err)
		}

		if memberRole != "creator" {
			t.Errorf("creator membership role = %q, want %q", memberRole, "creator")
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

// addOrgMember invites an existing user to the organization and
// accepts the invitation, establishing real org membership.
func addOrgMember(t *testing.T, orgID, inviterID int64, username string, userID int64) {
	t.Helper()

	err := repository.InviteOrganization(
		context.Background(),
		orgID,
		inviterID,
		username,
		"member",
	)
	if err != nil {
		t.Fatalf("invite fixture org member: %v", err)
	}

	var invitationID int64

	err = store.DB.QueryRow(
		context.Background(),
		`SELECT id FROM organization_invitations
		 WHERE organization_id = $1 AND user_id = $2 AND status = 'pending'`,
		orgID,
		userID,
	).Scan(&invitationID)
	if err != nil {
		t.Fatalf("load fixture invitation: %v", err)
	}

	if err := repository.AcceptInvitation(
		context.Background(),
		userID,
		invitationID,
	); err != nil {
		t.Fatalf("accept fixture invitation: %v", err)
	}
}

// addTeamMember adds an existing organization member to a team with
// the given role through the repository.
func addTeamMember(t *testing.T, teamID, callerID, userID int64, role string) {
	t.Helper()

	err := repository.AddNewMemberToTeam(
		context.Background(),
		teamID,
		userID,
		role,
		callerID,
	)
	if err != nil {
		t.Fatalf("add fixture team member: %v", err)
	}
}

func TestAddNewMemberToTeamHandlerIntegration(t *testing.T) {
	requireDB(t)

	ownerID := insertUser(t, "org_owner", "owner@strata.dev")
	orgMemberID := insertUser(t, "org_member", "member@strata.dev")
	outsiderID := insertUser(t, "outsider", "outsider@strata.dev")

	orgID := insertOrg(t, ownerID, "add-member-org")
	teamID := insertTeam(t, orgID, ownerID, "add-member-team")

	orgIDParam := strconv.FormatInt(orgID, 10)
	teamIDParam := strconv.FormatInt(teamID, 10)

	addOrgMember(t, orgID, ownerID, "org_member", orgMemberID)

	t.Run("the creator adds an org member as admin", func(t *testing.T) {
		rec := callAddMember(t, ownerID, orgIDParam, teamIDParam,
			`{"username":"org_member","role":"admin"}`)

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusCreated, rec.Body.String())
		}

		var res struct {
			Message string `json:"message"`
		}
		decodeResponse(t, rec, &res)

		if res.Message != "team_member_added" {
			t.Errorf("message = %q, want %q", res.Message, "team_member_added")
		}

		var role string

		err := store.DB.QueryRow(
			context.Background(),
			`SELECT role FROM team_members WHERE team_id = $1 AND user_id = $2`,
			teamID,
			orgMemberID,
		).Scan(&role)
		if err != nil {
			t.Fatalf("load team membership: %v", err)
		}

		if role != "admin" {
			t.Errorf("team membership role = %q, want %q", role, "admin")
		}
	})

	t.Run("a duplicate member is rejected", func(t *testing.T) {
		rec := callAddMember(t, ownerID, orgIDParam, teamIDParam,
			`{"username":"org_member","role":"member"}`)

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

	t.Run("an outsider caller is denied without a hint", func(t *testing.T) {
		rec := callAddMember(t, outsiderID, orgIDParam, teamIDParam,
			`{"username":"outsider","role":"member"}`)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusForbidden, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "team_access_denied" {
			t.Errorf("error = %q, want %q", res.Error, "team_access_denied")
		}
	})

	t.Run("a user outside the organization cannot be added", func(t *testing.T) {
		rec := callAddMember(t, ownerID, orgIDParam, teamIDParam,
			`{"username":"outsider","role":"member"}`)

		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusConflict, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "user_not_organization_member" {
			t.Errorf("error = %q, want %q", res.Error, "user_not_organization_member")
		}
	})

	t.Run("unknown username", func(t *testing.T) {
		rec := callAddMember(t, ownerID, orgIDParam, teamIDParam,
			`{"username":"ghost_user","role":"member"}`)

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

	t.Run("unknown team is indistinguishable from denied", func(t *testing.T) {
		rec := callAddMember(t, ownerID, orgIDParam, "424242",
			`{"username":"org_member","role":"member"}`)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusForbidden, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "team_access_denied" {
			t.Errorf("error = %q, want %q", res.Error, "team_access_denied")
		}
	})

	t.Run("invalid team id", func(t *testing.T) {
		rec := callAddMember(t, ownerID, orgIDParam, "not-a-number", "")

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
}

func TestListOrganizationTeamsHandlerIntegration(t *testing.T) {
	requireDB(t)

	ownerID := insertUser(t, "org_owner", "owner@strata.dev")
	memberID := insertUser(t, "org_member", "member@strata.dev")
	outsiderID := insertUser(t, "outsider", "outsider@strata.dev")

	orgID := insertOrg(t, ownerID, "list-teams-org")
	insertTeam(t, orgID, ownerID, "backend")
	insertTeam(t, orgID, ownerID, "frontend")

	emptyOrgID := insertOrg(t, ownerID, "empty-teams-org")

	addOrgMember(t, orgID, ownerID, "org_member", memberID)

	t.Run("a member sees the teams", func(t *testing.T) {
		rec := callListTeams(t, memberID, strconv.FormatInt(orgID, 10))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusOK, rec.Body.String())
		}

		var res struct {
			Teams []models.OrganizationTeam `json:"teams"`
		}
		decodeResponse(t, rec, &res)

		if len(res.Teams) != 2 {
			t.Fatalf("teams = %d, want 2: %+v", len(res.Teams), res.Teams)
		}

		identifiers := map[string]bool{}

		for _, team := range res.Teams {
			if team.ID <= 0 || team.Name == "" || team.CreatedAt.IsZero() {
				t.Errorf("team %q is missing fields: %+v", team.Identifier, team)
			}

			identifiers[team.Identifier] = true
		}

		if !identifiers["backend"] || !identifiers["frontend"] {
			t.Errorf("expected backend and frontend, got %v", identifiers)
		}
	})

	t.Run("an organization without teams lists an empty array", func(t *testing.T) {
		rec := callListTeams(t, ownerID, strconv.FormatInt(emptyOrgID, 10))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusOK, rec.Body.String())
		}

		want := "{\"teams\":[]}\n"

		if rec.Body.String() != want {
			t.Errorf("body = %s, want %s", rec.Body.String(), want)
		}
	})

	t.Run("a non-member gets no hint that the organization exists", func(t *testing.T) {
		rec := callListTeams(t, outsiderID, strconv.FormatInt(orgID, 10))

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
		rec := callListTeams(t, ownerID, "424242")

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
		rec := callListTeams(t, ownerID, "not-a-number")

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

func TestRemoveMemberFromTeamHandlerIntegration(t *testing.T) {
	requireDB(t)

	ownerID := insertUser(t, "org_owner", "owner@strata.dev")
	adminID := insertUser(t, "team_admin", "teamadmin@strata.dev")
	memberID := insertUser(t, "team_member", "teammember@strata.dev")
	outsiderID := insertUser(t, "outsider", "outsider@strata.dev")

	// freshTeam creates an organization with a team where adminID is
	// an admin and memberID is a member, so each mutating subtest
	// works on its own rows.
	freshTeam := func(t *testing.T, slug string) (orgID, teamID int64) {
		t.Helper()

		orgID = insertOrg(t, ownerID, slug)
		teamID = insertTeam(t, orgID, ownerID, slug+"-team")

		addOrgMember(t, orgID, ownerID, "team_admin", adminID)
		addOrgMember(t, orgID, ownerID, "team_member", memberID)

		addTeamMember(t, teamID, ownerID, adminID, "admin")
		addTeamMember(t, teamID, ownerID, memberID, "member")

		return orgID, teamID
	}

	// countTeamMembers returns the number of members the team has.
	countTeamMembers := func(t *testing.T, teamID, userID int64) int {
		t.Helper()

		var count int

		err := store.DB.QueryRow(
			context.Background(),
			`SELECT COUNT(*) FROM team_members WHERE team_id = $1 AND user_id = $2`,
			teamID,
			userID,
		).Scan(&count)
		if err != nil {
			t.Fatalf("count team members: %v", err)
		}

		return count
	}

	t.Run("the creator removes an admin", func(t *testing.T) {
		orgID, teamID := freshTeam(t, "remove-admin")

		rec := callRemoveMember(t, ownerID,
			strconv.FormatInt(orgID, 10),
			strconv.FormatInt(teamID, 10),
			strconv.FormatInt(adminID, 10),
		)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusOK, rec.Body.String())
		}

		var res struct {
			Message string `json:"message"`
		}
		decodeResponse(t, rec, &res)

		if res.Message != "team_member_removed" {
			t.Errorf("message = %q, want %q", res.Message, "team_member_removed")
		}

		if count := countTeamMembers(t, teamID, adminID); count != 0 {
			t.Errorf("memberships left = %d, want 0", count)
		}
	})

	t.Run("an admin removes a member", func(t *testing.T) {
		orgID, teamID := freshTeam(t, "admin-removes")

		rec := callRemoveMember(t, adminID,
			strconv.FormatInt(orgID, 10),
			strconv.FormatInt(teamID, 10),
			strconv.FormatInt(memberID, 10),
		)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusOK, rec.Body.String())
		}

		if count := countTeamMembers(t, teamID, memberID); count != 0 {
			t.Errorf("memberships left = %d, want 0", count)
		}
	})

	t.Run("a plain member cannot remove anyone", func(t *testing.T) {
		orgID, teamID := freshTeam(t, "member-denied")

		rec := callRemoveMember(t, memberID,
			strconv.FormatInt(orgID, 10),
			strconv.FormatInt(teamID, 10),
			strconv.FormatInt(adminID, 10),
		)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusForbidden, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "team_access_denied" {
			t.Errorf("error = %q, want %q", res.Error, "team_access_denied")
		}
	})

	t.Run("an outsider is denied without a hint", func(t *testing.T) {
		orgID, teamID := freshTeam(t, "outsider-denied")

		rec := callRemoveMember(t, outsiderID,
			strconv.FormatInt(orgID, 10),
			strconv.FormatInt(teamID, 10),
			strconv.FormatInt(memberID, 10),
		)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusForbidden, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "team_access_denied" {
			t.Errorf("error = %q, want %q", res.Error, "team_access_denied")
		}
	})

	t.Run("the creator cannot be removed by an admin", func(t *testing.T) {
		orgID, teamID := freshTeam(t, "creator-protected")

		rec := callRemoveMember(t, adminID,
			strconv.FormatInt(orgID, 10),
			strconv.FormatInt(teamID, 10),
			strconv.FormatInt(ownerID, 10),
		)

		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusConflict, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "cannot_remove_creator" {
			t.Errorf("error = %q, want %q", res.Error, "cannot_remove_creator")
		}
	})

	t.Run("the creator cannot remove themselves", func(t *testing.T) {
		orgID, teamID := freshTeam(t, "creator-self")

		rec := callRemoveMember(t, ownerID,
			strconv.FormatInt(orgID, 10),
			strconv.FormatInt(teamID, 10),
			strconv.FormatInt(ownerID, 10),
		)

		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusConflict, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "cannot_remove_creator" {
			t.Errorf("error = %q, want %q", res.Error, "cannot_remove_creator")
		}
	})

	t.Run("an admin can remove themselves", func(t *testing.T) {
		orgID, teamID := freshTeam(t, "admin-self")

		rec := callRemoveMember(t, adminID,
			strconv.FormatInt(orgID, 10),
			strconv.FormatInt(teamID, 10),
			strconv.FormatInt(adminID, 10),
		)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusOK, rec.Body.String())
		}

		if count := countTeamMembers(t, teamID, adminID); count != 0 {
			t.Errorf("memberships left = %d, want 0", count)
		}
	})

	t.Run("removing a non-member", func(t *testing.T) {
		orgID, teamID := freshTeam(t, "non-member")

		// The outsider user exists but is not part of the team.
		rec := callRemoveMember(t, ownerID,
			strconv.FormatInt(orgID, 10),
			strconv.FormatInt(teamID, 10),
			strconv.FormatInt(outsiderID, 10),
		)

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

	t.Run("unknown team is indistinguishable from denied", func(t *testing.T) {
		orgID, _ := freshTeam(t, "unknown-team")

		rec := callRemoveMember(t, ownerID,
			strconv.FormatInt(orgID, 10),
			"424242",
			strconv.FormatInt(memberID, 10),
		)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusForbidden, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "team_access_denied" {
			t.Errorf("error = %q, want %q", res.Error, "team_access_denied")
		}
	})

	t.Run("invalid member id", func(t *testing.T) {
		orgID, teamID := freshTeam(t, "invalid-member")

		rec := callRemoveMember(t, ownerID,
			strconv.FormatInt(orgID, 10),
			strconv.FormatInt(teamID, 10),
			"not-a-number",
		)

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

	t.Run("invalid team id", func(t *testing.T) {
		orgID, _ := freshTeam(t, "invalid-team")

		rec := callRemoveMember(t, ownerID,
			strconv.FormatInt(orgID, 10),
			"not-a-number",
			strconv.FormatInt(memberID, 10),
		)

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
}
