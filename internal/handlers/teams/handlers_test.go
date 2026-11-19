package teams

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coderianx/Strata/internal/middlewares"
	"github.com/coderianx/Strata/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "teams-test-secret"

// callHandler invokes a handler directly, without the auth
// middleware.
func callHandler(
	t *testing.T,
	handler http.HandlerFunc,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler(rec, req)

	return rec
}

// teamsRouter mounts the team handlers behind the middleware with
// route patterns, so the URL parameters are populated exactly like in
// production.
func teamsRouter() http.Handler {
	router := chi.NewRouter()

	router.Route("/api/v1/organizations", func(r chi.Router) {
		r.With(middlewares.Auth).Get("/{organization_id}/teams", ListOrganizationTeamsHandler)
		r.With(middlewares.Auth).Post("/{organization_id}/teams", CreateNewTeamHandler)
		r.With(middlewares.Auth).Delete("/{organization_id}/teams/{team_id}", DeleteTeamHandler)
		r.With(middlewares.Auth).Post("/{organization_id}/teams/{team_id}/members", AddNewMemberToTeamHandler)
		r.With(middlewares.Auth).Delete("/{organization_id}/teams/{team_id}/members/{member_id}", RemoveMemberFromTeamHandler)
	})

	return router
}

// callCreateTeam sends an authenticated team creation request through
// the router. The organization id only has to parse: these tests
// never reach the database.
func callCreateTeam(
	t *testing.T,
	userID int64,
	organizationID, body string,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/organizations/"+organizationID+"/teams",
		strings.NewReader(body),
	)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, testSecret, userID))

	rec := httptest.NewRecorder()

	teamsRouter().ServeHTTP(rec, req)

	return rec
}

// callDeleteTeam sends an authenticated team deletion request through
// the router. The ids only have to parse: these tests never reach
// the database.
func callDeleteTeam(
	t *testing.T,
	userID int64,
	organizationID, teamID string,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/organizations/"+organizationID+"/teams/"+teamID,
		nil,
	)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, testSecret, userID))

	rec := httptest.NewRecorder()

	teamsRouter().ServeHTTP(rec, req)

	return rec
}

// makeToken signs an HS256 access token for the given user,
// mirroring the tokens the auth handlers issue after login.
func makeToken(t *testing.T, secret string, userID int64) string {
	t.Helper()

	claims := models.Claims{
		UserID:   userID,
		Username: "kodcu_42",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).
		SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign test token: %v", err)
	}

	return signed
}

// decodeResponse asserts the response is JSON and decodes it into got.
func decodeResponse(t *testing.T, rec *httptest.ResponseRecorder, got any) {
	t.Helper()

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	if err := json.NewDecoder(rec.Body).Decode(got); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
}

// The tests in this file cover the handlers' request handling up to,
// but not including, the database calls. The database-backed paths
// live in handlers_integration_test.go behind the "integration" build
// tag.

func TestCreateNewTeamHandler(t *testing.T) {
	t.Run("request without the middleware", func(t *testing.T) {
		// Mounting the handler unprotected must fail safely instead
		// of serving anonymous requests.
		rec := callHandler(t, CreateNewTeamHandler, "")

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "unauthorized" {
			t.Errorf("error = %q, want %q", res.Error, "unauthorized")
		}
	})

	t.Run("invalid organization id", func(t *testing.T) {
		// A route the pattern cannot parse must fail before anything
		// else is read.
		t.Setenv("JWT_SECRET", testSecret)

		rec := callCreateTeam(t, 42, "not-a-number", "")

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

	t.Run("malformed JSON", func(t *testing.T) {
		t.Setenv("JWT_SECRET", testSecret)

		rec := callCreateTeam(t, 42, "42", `{"name":`)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "invalid_request" {
			t.Errorf("error = %q, want %q", res.Error, "invalid_request")
		}
	})

	t.Run("validation failures", func(t *testing.T) {
		t.Setenv("JWT_SECRET", testSecret)

		tests := []struct {
			name      string
			body      string
			wantError string
		}{
			{"missing name", `{"identifier":"backend-team"}`, "name is required"},
			{"missing identifier", `{"name":"Backend Team"}`, "identifier must be at least 3 characters"},
			{"ends with hyphen", `{"name":"Backend Team","identifier":"backend-"}`, "identifier cannot end with a hyphen"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				rec := callCreateTeam(t, 42, "42", tt.body)

				if rec.Code != http.StatusBadRequest {
					t.Errorf("status = %d, want %d; body = %s",
						rec.Code, http.StatusBadRequest, rec.Body.String())
				}

				var res struct {
					Error string `json:"error"`
				}
				decodeResponse(t, rec, &res)

				if res.Error != tt.wantError {
					t.Errorf("error = %q, want %q", res.Error, tt.wantError)
				}
			})
		}
	})
}

func TestDeleteTeamHandler(t *testing.T) {
	t.Run("request without the middleware", func(t *testing.T) {
		rec := callHandler(t, DeleteTeamHandler, "")

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "unauthorized" {
			t.Errorf("error = %q, want %q", res.Error, "unauthorized")
		}
	})

	t.Run("invalid organization id", func(t *testing.T) {
		t.Setenv("JWT_SECRET", testSecret)

		rec := callDeleteTeam(t, 42, "not-a-number", "7")

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

	t.Run("invalid team id", func(t *testing.T) {
		t.Setenv("JWT_SECRET", testSecret)

		rec := callDeleteTeam(t, 42, "42", "not-a-number")

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

// callListTeams requests an organization's team list through the
// router. The organization id only has to parse: these tests never
// reach the database.
func callListTeams(
	t *testing.T,
	userID int64,
	organizationID string,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/organizations/"+organizationID+"/teams",
		nil,
	)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, testSecret, userID))

	rec := httptest.NewRecorder()

	teamsRouter().ServeHTTP(rec, req)

	return rec
}

// callAddMember sends an authenticated add-team-member request
// through the router. The ids only have to parse: these tests never
// reach the database.
func callAddMember(
	t *testing.T,
	userID int64,
	organizationID, teamID, body string,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/organizations/"+organizationID+"/teams/"+teamID+"/members",
		strings.NewReader(body),
	)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, testSecret, userID))

	rec := httptest.NewRecorder()

	teamsRouter().ServeHTTP(rec, req)

	return rec
}

// callRemoveMember sends an authenticated remove-team-member request
// through the router. The ids only have to parse: these tests never
// reach the database.
func callRemoveMember(
	t *testing.T,
	userID int64,
	organizationID, teamID, memberID string,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/organizations/"+organizationID+"/teams/"+teamID+"/members/"+memberID,
		nil,
	)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, testSecret, userID))

	rec := httptest.NewRecorder()

	teamsRouter().ServeHTTP(rec, req)

	return rec
}

func TestAddNewMemberToTeamHandler(t *testing.T) {
	t.Run("request without the middleware", func(t *testing.T) {
		// Mounting the handler unprotected must fail safely instead
		// of serving anonymous requests.
		rec := callHandler(t, AddNewMemberToTeamHandler, "")

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "unauthorized" {
			t.Errorf("error = %q, want %q", res.Error, "unauthorized")
		}
	})

	t.Run("invalid organization id", func(t *testing.T) {
		t.Setenv("JWT_SECRET", testSecret)

		rec := callAddMember(t, 42, "not-a-number", "7", "")

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

	t.Run("invalid team id", func(t *testing.T) {
		t.Setenv("JWT_SECRET", testSecret)

		rec := callAddMember(t, 42, "42", "not-a-number", "")

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

	t.Run("malformed JSON", func(t *testing.T) {
		t.Setenv("JWT_SECRET", testSecret)

		rec := callAddMember(t, 42, "42", "7", `{"username":`)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "invalid_request" {
			t.Errorf("error = %q, want %q", res.Error, "invalid_request")
		}
	})

	t.Run("validation failures", func(t *testing.T) {
		t.Setenv("JWT_SECRET", testSecret)

		tests := []struct {
			name      string
			body      string
			wantError string
		}{
			{"missing username", `{"role":"member"}`, "username is required"},
			{"unknown role", `{"username":"kodcu_42","role":"creator"}`, "role must be one of: member, admin"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				rec := callAddMember(t, 42, "42", "7", tt.body)

				if rec.Code != http.StatusBadRequest {
					t.Errorf("status = %d, want %d; body = %s",
						rec.Code, http.StatusBadRequest, rec.Body.String())
				}

				var res struct {
					Error string `json:"error"`
				}
				decodeResponse(t, rec, &res)

				if res.Error != tt.wantError {
					t.Errorf("error = %q, want %q", res.Error, tt.wantError)
				}
			})
		}
	})
}

func TestListOrganizationTeamsHandler(t *testing.T) {
	t.Run("request without the middleware", func(t *testing.T) {
		rec := callHandler(t, ListOrganizationTeamsHandler, "")

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "unauthorized" {
			t.Errorf("error = %q, want %q", res.Error, "unauthorized")
		}
	})

	t.Run("invalid organization id", func(t *testing.T) {
		// A route the pattern cannot parse must fail before anything
		// else is read.
		t.Setenv("JWT_SECRET", testSecret)

		rec := callListTeams(t, 42, "not-a-number")

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

func TestRemoveMemberFromTeamHandler(t *testing.T) {
	t.Run("request without the middleware", func(t *testing.T) {
		rec := callHandler(t, RemoveMemberFromTeamHandler, "")

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "unauthorized" {
			t.Errorf("error = %q, want %q", res.Error, "unauthorized")
		}
	})

	t.Run("invalid organization id", func(t *testing.T) {
		t.Setenv("JWT_SECRET", testSecret)

		rec := callRemoveMember(t, 42, "not-a-number", "7", "9")

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

	t.Run("invalid team id", func(t *testing.T) {
		t.Setenv("JWT_SECRET", testSecret)

		rec := callRemoveMember(t, 42, "42", "not-a-number", "9")

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

	t.Run("invalid member id", func(t *testing.T) {
		t.Setenv("JWT_SECRET", testSecret)

		rec := callRemoveMember(t, 42, "42", "7", "not-a-number")

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
}
