package middlewares

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/coderianx/Strata/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "mw-test-secret"

// newTestRouter mounts Auth exactly the way the application will:
// per-route with chi's r.With. The protected handler echoes the user
// attached to the request context.
func newTestRouter() http.Handler {
	router := chi.NewRouter()

	router.Route("/api/v1", func(r chi.Router) {
		r.With(Auth).Get("/me", func(w http.ResponseWriter, r *http.Request) {
			user, ok := UserFromContext(r.Context())
			if !ok {
				http.Error(w, "no user in context", http.StatusInternalServerError)

				return
			}

			w.Header().Set("Content-Type", "application/json")

			json.NewEncoder(w).Encode(map[string]any{
				"id":       user.ID,
				"username": user.Username,
			})
		})
	})

	return router
}

// makeToken signs an HS256 access token the same way generateJWT
// issues them.
func makeToken(t *testing.T, secret string, expires time.Time) string {
	t.Helper()

	claims := models.Claims{
		UserID:   42,
		Username: "kodcu_42",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(42, 10),
			ExpiresAt: jwt.NewNumericDate(expires),
		},
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).
		SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign test token: %v", err)
	}

	return signed
}

// callAuth requests the protected route with the given header value.
func callAuth(authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", authHeader)

	rec := httptest.NewRecorder()

	newTestRouter().ServeHTTP(rec, req)

	return rec
}

func TestUserFromContextWithoutAuth(t *testing.T) {
	// Contexts that never passed through Auth must not claim a user.
	if _, ok := UserFromContext(context.Background()); ok {
		t.Error("UserFromContext() = true without Auth, want false")
	}
}

func TestAuth(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
		wantBody   string
	}{
		{
			"valid token",
			"Bearer " + makeToken(t, testSecret, time.Now().Add(15*time.Minute)),
			http.StatusOK,
			"{\"id\":42,\"username\":\"kodcu_42\"}\n",
		},
		{
			// RFC 7235: auth schemes are case-insensitive.
			"lowercase scheme with padding",
			"bearer    " + makeToken(t, testSecret, time.Now().Add(15*time.Minute)),
			http.StatusOK,
			"{\"id\":42,\"username\":\"kodcu_42\"}\n",
		},
		{
			"missing Authorization header",
			"",
			http.StatusUnauthorized,
			"{\"error\":\"invalid_token\"}\n",
		},
		{
			"wrong scheme",
			"Basic dXNlcjpwYXNz",
			http.StatusUnauthorized,
			"{\"error\":\"invalid_token\"}\n",
		},
		{
			"scheme without token",
			"Bearer",
			http.StatusUnauthorized,
			"{\"error\":\"invalid_token\"}\n",
		},
		{
			"empty token",
			"Bearer ",
			http.StatusUnauthorized,
			"{\"error\":\"invalid_token\"}\n",
		},
		{
			"whitespace only token",
			"Bearer    ",
			http.StatusUnauthorized,
			"{\"error\":\"invalid_token\"}\n",
		},
		{
			"malformed token",
			"Bearer not-a-jwt",
			http.StatusUnauthorized,
			"{\"error\":\"invalid_token\"}\n",
		},
		{
			"wrong signing secret",
			"Bearer " + makeToken(t, "other-secret", time.Now().Add(15*time.Minute)),
			http.StatusUnauthorized,
			"{\"error\":\"invalid_token\"}\n",
		},
		{
			"expired token",
			"Bearer " + makeToken(t, testSecret, time.Now().Add(-15*time.Minute)),
			http.StatusUnauthorized,
			"{\"error\":\"invalid_token\"}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := callAuth(tt.authHeader)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s",
					rec.Code, tt.wantStatus, rec.Body.String())
			}

			if rec.Body.String() != tt.wantBody {
				t.Errorf("body = %s, want %s", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestAuthRejectsUnsignedTokens(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)

	// alg=none tokens carry no signature; they must never
	// authenticate even though their structure is well-formed.
	claims := models.Claims{
		UserID:   42,
		Username: "kodcu_42",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "42",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).
		SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign alg-none token: %v", err)
	}

	rec := callAuth("Bearer " + signed)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("alg-none token status = %d, want %d; body = %s",
			rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestAuthMissingSecret(t *testing.T) {
	// The secret check runs before parsing, so any bearer value
	// reaches the internal error path.
	t.Setenv("JWT_SECRET", "")

	rec := callAuth("Bearer any-token")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	want := "{\"error\":\"internal_error\"}\n"
	if rec.Body.String() != want {
		t.Errorf("body = %s, want %s", rec.Body.String(), want)
	}
}
