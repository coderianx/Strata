package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coderianx/Strata/internal/middlewares"
)

// callHandler invokes a handler with a JSON request body and returns
// the recorded response.
func callHandler(
	t *testing.T,
	handler func(http.ResponseWriter, *http.Request),
	body string,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler(rec, req)

	return rec
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
// but not including, the database calls: JSON decoding, normalization
// and validation. The database-backed paths need a PostgreSQL
// instance and live in handlers_integration_test.go behind the
// "integration" build tag.

func TestRegisterHandler(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantError  string
	}{
		{
			"malformed JSON",
			`{"name":`,
			http.StatusBadRequest,
			"invalid_request",
		},
		{
			"empty body",
			``,
			http.StatusBadRequest,
			"invalid_request",
		},
		{
			"array instead of object",
			`[]`,
			http.StatusBadRequest,
			"invalid_request",
		},
		{
			"null body",
			`null`,
			http.StatusBadRequest,
			"name is required",
		},
		{
			"missing name",
			`{"username":"kodcu_42","email":"kodcu@strata.dev","password":"sifre1234"}`,
			http.StatusBadRequest,
			"name is required",
		},
		{
			"missing username",
			`{"name":"Kodcu","email":"kodcu@strata.dev","password":"sifre1234"}`,
			http.StatusBadRequest,
			"username must be at least 3 characters",
		},
		{
			"too long username",
			fmt.Sprintf(
				`{"name":"Kodcu","username":%q,"email":"kodcu@strata.dev","password":"sifre1234"}`,
				strings.Repeat("a", 21),
			),
			http.StatusBadRequest,
			"username must be at most 20 characters",
		},
		{
			"uppercase username",
			`{"name":"Kodcu","username":"Kodcu","email":"kodcu@strata.dev","password":"sifre1234"}`,
			http.StatusBadRequest,
			"username can only contain lowercase letters, numbers and underscores",
		},
		{
			"username starts with a digit",
			`{"name":"Kodcu","username":"1kodcu","email":"kodcu@strata.dev","password":"sifre1234"}`,
			http.StatusBadRequest,
			"username must start with a letter",
		},
		{
			"reserved username",
			`{"name":"Kodcu","username":"admin","email":"kodcu@strata.dev","password":"sifre1234"}`,
			http.StatusBadRequest,
			"this username is reserved",
		},
		{
			"invalid email",
			`{"name":"Kodcu","username":"kodcu_42","email":"kodcu@strata","password":"sifre1234"}`,
			http.StatusBadRequest,
			"email is invalid",
		},
		{
			"short password",
			`{"name":"Kodcu","username":"kodcu_42","email":"kodcu@strata.dev","password":"sifre12"}`,
			http.StatusBadRequest,
			"password must be at least 8 characters",
		},
		{
			"oversized password",
			fmt.Sprintf(
				`{"name":"Kodcu","username":"kodcu_42","email":"kodcu@strata.dev","password":"%s"}`,
				strings.Repeat("a", 73),
			),
			http.StatusBadRequest,
			"password must be at most 72 bytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := callHandler(t, RegisterHandler, tt.body)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s",
					rec.Code, tt.wantStatus, rec.Body.String())
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
}

func TestLoginHandler(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantError  string
	}{
		{
			"malformed JSON",
			`{"email":`,
			http.StatusBadRequest,
			"invalid_request",
		},
		{
			"empty body",
			``,
			http.StatusBadRequest,
			"invalid_request",
		},
		{
			"missing password",
			`{"email":"kodcu@strata.dev"}`,
			http.StatusBadRequest,
			"email and password are required",
		},
		{
			"missing email",
			`{"password":"sifre1234"}`,
			http.StatusBadRequest,
			"email and password are required",
		},
		{
			"empty object",
			`{}`,
			http.StatusBadRequest,
			"email and password are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := callHandler(t, LoginHandler, tt.body)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s",
					rec.Code, tt.wantStatus, rec.Body.String())
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
}

func TestRefreshHandler(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantError  string
	}{
		{
			"malformed JSON",
			`{"refreshToken":`,
			http.StatusBadRequest,
			"invalid_request",
		},
		{
			"empty body",
			``,
			http.StatusBadRequest,
			"invalid_request",
		},
		{
			"missing token",
			`{}`,
			http.StatusBadRequest,
			"refreshToken is required",
		},
		{
			"whitespace only token",
			`{"refreshToken":"   "}`,
			http.StatusBadRequest,
			"refreshToken is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := callHandler(t, RefreshHandler, tt.body)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s",
					rec.Code, tt.wantStatus, rec.Body.String())
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
}

func TestGetMeHandler(t *testing.T) {
	t.Run("authenticated request", func(t *testing.T) {
		t.Setenv("JWT_SECRET", "test-secret")

		// Drive the full production path: Auth validates the access
		// token and attaches the user; GetMeHandler echoes it.
		accessToken, err := generateJWT(42, "kodcu_42")

		if err != nil {
			t.Fatalf("generateJWT() = %v, want nil", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)

		rec := httptest.NewRecorder()

		middlewares.Auth(http.HandlerFunc(GetMeHandler)).ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body = %s",
				rec.Code, http.StatusOK, rec.Body.String())
		}

		want := "{\"id\":42,\"username\":\"kodcu_42\"}\n"

		if rec.Body.String() != want {
			t.Errorf("body = %s, want %s", rec.Body.String(), want)
		}
	})

	t.Run("request without the middleware", func(t *testing.T) {
		// Mounting GetMeHandler unprotected must fail safely instead
		// of serving anonymous requests.
		rec := callHandler(t, GetMeHandler, "")

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}

		want := "{\"error\":\"invalid_token\"}\n"

		if rec.Body.String() != want {
			t.Errorf("body = %s, want %s", rec.Body.String(), want)
		}
	})
}
