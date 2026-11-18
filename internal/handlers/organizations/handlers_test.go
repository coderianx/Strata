package organizations

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

const testSecret = "org-test-secret"

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

// callAuthed invokes the handler through the auth middleware with an
// access token for userID, exactly like a routed request would.
func callAuthed(
	t *testing.T,
	handler http.HandlerFunc,
	userID int64,
	method, target, body string,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+makeToken(t, testSecret, userID))

	rec := httptest.NewRecorder()

	middlewares.Auth(handler).ServeHTTP(rec, req)

	return rec
}

// inviteRouter mounts the invite handler behind the middleware with
// a route pattern, so the organization_id URL parameter is populated
// exactly like in production.
func inviteRouter() http.Handler {
	router := chi.NewRouter()

	router.Route("/api/v1/organizations", func(r chi.Router) {
		r.With(middlewares.Auth).Post("/{organization_id}/invite", InviteUserToOrganizationHandler)
	})

	return router
}

// callInvite sends an authenticated invite request through the
// router. The organization id only has to parse: these tests never
// reach the database.
func callInvite(t *testing.T, userID int64, organizationID, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/organizations/"+organizationID+"/invite",
		strings.NewReader(body),
	)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, testSecret, userID))

	rec := httptest.NewRecorder()

	inviteRouter().ServeHTTP(rec, req)

	return rec
}

// invitationRouter mounts the accept and reject handlers behind the
// middleware with route patterns, so the invitation_id URL parameter
// is populated exactly like in production.
func invitationRouter() http.Handler {
	router := chi.NewRouter()

	router.Route("/api/v1/invitations", func(r chi.Router) {
		r.With(middlewares.Auth).Post("/{invitation_id}/accept", AcceptInvitationHandler)
		r.With(middlewares.Auth).Post("/{invitation_id}/reject", RejectInvitationHandler)
	})

	return router
}

// callInvitation sends an authenticated accept or reject request
// through the router. The invitation id only has to parse: these
// tests never reach the database.
func callInvitation(
	t *testing.T,
	action string,
	userID int64,
	invitationID string,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/invitations/"+invitationID+"/"+action,
		nil,
	)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, testSecret, userID))

	rec := httptest.NewRecorder()

	invitationRouter().ServeHTTP(rec, req)

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

func TestCreateNewOrganizationHandler(t *testing.T) {
	t.Run("request without the middleware", func(t *testing.T) {
		// Mounting the handler unprotected must fail safely instead
		// of serving anonymous requests.
		rec := callHandler(t, CreateNewOrganizationHandler, "")

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

	t.Run("malformed JSON", func(t *testing.T) {
		t.Setenv("JWT_SECRET", testSecret)

		rec := callAuthed(t, CreateNewOrganizationHandler, 42, http.MethodPost, "/", `{"name":`)

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
			{"missing name", `{"slug":"coderian-studio"}`, "name is required"},
			{"missing slug", `{"name":"Coderian Studio"}`, "slug must be at least 3 characters"},
			{"reserved slug", `{"name":"Coderian Studio","slug":"admin"}`, "this slug is reserved"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				rec := callAuthed(t, CreateNewOrganizationHandler, 42, http.MethodPost, "/", tt.body)

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

func TestDeleteOrganization(t *testing.T) {
	t.Run("request without the middleware", func(t *testing.T) {
		rec := callHandler(t, DeleteOrganization, "")

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
		// Direct handler call: no chi route context, so the URL
		// parameter is empty and parsing must fail.
		t.Setenv("JWT_SECRET", testSecret)

		rec := callAuthed(t, DeleteOrganization, 42, http.MethodDelete, "/", "")

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

func TestInviteUserToOrganizationHandler(t *testing.T) {
	t.Run("request without the middleware", func(t *testing.T) {
		// Mounting the handler unprotected must fail safely instead
		// of serving anonymous requests.
		rec := callHandler(t, InviteUserToOrganizationHandler, "")

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

		rec := callInvite(t, 42, "not-a-number", "")

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

		rec := callInvite(t, 42, "42", `{"username":`)

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
			{"unknown role", `{"username":"kodcu_42","role":"superadmin"}`, "role must be one of: member, owner"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				rec := callInvite(t, 42, "42", tt.body)

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

func TestListMyInvitationsHandler(t *testing.T) {
	t.Run("request without the middleware", func(t *testing.T) {
		rec := callHandler(t, ListMyInvitationsHandler, "")

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
}

func TestAcceptInvitationHandler(t *testing.T) {
	t.Run("request without the middleware", func(t *testing.T) {
		rec := callHandler(t, AcceptInvitationHandler, "")

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

	t.Run("invalid invitation id", func(t *testing.T) {
		// A route the pattern cannot parse must fail before anything
		// else is read.
		t.Setenv("JWT_SECRET", testSecret)

		rec := callInvitation(t, "accept", 42, "not-a-number")

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

func TestRejectInvitationHandler(t *testing.T) {
	t.Run("request without the middleware", func(t *testing.T) {
		rec := callHandler(t, RejectInvitationHandler, "")

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

	t.Run("invalid invitation id", func(t *testing.T) {
		// A route the pattern cannot parse must fail before anything
		// else is read.
		t.Setenv("JWT_SECRET", testSecret)

		rec := callInvitation(t, "reject", 42, "not-a-number")

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

// membersRouter mounts the members and remove handlers behind the
// middleware with route patterns, so the URL parameters are
// populated exactly like in production.
func membersRouter() http.Handler {
	router := chi.NewRouter()

	router.Route("/api/v1/organizations", func(r chi.Router) {
		r.With(middlewares.Auth).Get("/{organization_id}/members", OrganizationMembersHandler)
		r.With(middlewares.Auth).Delete("/{organization_id}/members/{member_id}", RemoveMemberHandler)
	})

	return router
}

// callMembers requests an organization's member list through the
// router. The organization id only has to parse: these tests never
// reach the database.
func callMembers(
	t *testing.T,
	userID int64,
	organizationID string,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/organizations/"+organizationID+"/members",
		nil,
	)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, testSecret, userID))

	rec := httptest.NewRecorder()

	membersRouter().ServeHTTP(rec, req)

	return rec
}

// callRemove sends an authenticated remove-member request through
// the router. The ids only have to parse: these tests never reach
// the database.
func callRemove(
	t *testing.T,
	userID int64,
	organizationID, memberID string,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/organizations/"+organizationID+"/members/"+memberID,
		nil,
	)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, testSecret, userID))

	rec := httptest.NewRecorder()

	membersRouter().ServeHTTP(rec, req)

	return rec
}

func TestGetMyOrganizationsHandler(t *testing.T) {
	t.Run("request without the middleware", func(t *testing.T) {
		// Mounting the handler unprotected must fail safely instead
		// of serving anonymous requests.
		rec := callHandler(t, GetMyOrganizationsHandler, "")

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
}

func TestOrganizationMembersHandler(t *testing.T) {
	t.Run("request without the middleware", func(t *testing.T) {
		rec := callHandler(t, OrganizationMembersHandler, "")

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

		rec := callMembers(t, 42, "not-a-number")

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

func TestRemoveMemberHandler(t *testing.T) {
	t.Run("request without the middleware", func(t *testing.T) {
		rec := callHandler(t, RemoveMemberHandler, "")

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

		rec := callRemove(t, 42, "not-a-number", "7")

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

	t.Run("invalid member id", func(t *testing.T) {
		t.Setenv("JWT_SECRET", testSecret)

		rec := callRemove(t, 42, "42", "not-a-number")

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
