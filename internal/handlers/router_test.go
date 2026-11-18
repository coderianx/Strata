package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPIRouter(t *testing.T) {
	router := APIRouter()

	if router == nil {
		t.Fatal("APIRouter() = nil, want a usable router")
	}

	// Anything that is not a registered route falls through to the
	// default NotFound handler.
	req := httptest.NewRequest(http.MethodGet, "/no-such-route", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown route status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestAPIRouterRoutes pins the wiring of the mounted routes: each
// path must reach its real handler with the JSON contract pinned by
// the tests in internal/handlers/auth.
func TestAPIRouterRoutes(t *testing.T) {
	router := APIRouter()

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantBody   string // empty means the body is not checked
	}{
		{
			"health",
			http.MethodGet,
			"/health",
			http.StatusOK,
			"{\"status\":\"ok\"}\n",
		},
		{
			"register",
			http.MethodPost,
			"/api/v1/auth/register",
			http.StatusBadRequest,
			"{\"error\":\"invalid_request\"}\n",
		},
		{
			"login",
			http.MethodPost,
			"/api/v1/auth/login",
			http.StatusBadRequest,
			"{\"error\":\"invalid_request\"}\n",
		},
		{
			"refresh",
			http.MethodPost,
			"/api/v1/auth/refresh",
			http.StatusBadRequest,
			"{\"error\":\"invalid_request\"}\n",
		},
		{
			"me without token",
			http.MethodGet,
			"/api/v1/auth/me",
			http.StatusUnauthorized,
			"{\"error\":\"invalid_token\"}\n",
		},
		{
			"register rejects other methods",
			http.MethodGet,
			"/api/v1/auth/register",
			http.StatusMethodNotAllowed,
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(""))
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("%s %s status = %d, want %d",
					tt.method, tt.path, rec.Code, tt.wantStatus)
			}

			if tt.wantBody != "" && rec.Body.String() != tt.wantBody {
				t.Errorf("%s %s body = %s, want %s",
					tt.method, tt.path, rec.Body.String(), tt.wantBody)
			}
		})
	}
}
