// Package middlewares provides HTTP middlewares for Strata.
package middlewares

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/coderianx/Strata/internal/config"
	"github.com/coderianx/Strata/internal/models"
	"github.com/golang-jwt/jwt/v5"
)

// userContextKey is the context key Auth attaches the authenticated
// user under. An unexported struct type cannot collide with keys
// defined by other packages.
type userContextKey struct{}

// User is the authenticated identity Auth extracts from the access
// token and attaches to the request context.
type User struct {
	ID       int64
	Username string
}

// UserFromContext returns the user Auth attached to the request. The
// second return is false when the request never passed through the
// middleware, i.e. the handler is unprotected.
func UserFromContext(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(userContextKey{}).(User)

	return user, ok
}

// Auth validates the request's Bearer access token and attaches the
// authenticated user to the request context. It is a chi middleware,
// meant to be mounted per-route with chi's r.With:
//
//	r.With(middlewares.Auth).Get("/me", handler)
//
// Missing, malformed, expired or wrongly signed tokens all get the
// same 401 {"error":"invalid_token"} response; the reason is never
// revealed, mirroring how LoginHandler treats bad credentials.
func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// HTTP auth schemes are case-insensitive (RFC 7235).
		scheme, tokenString, found := strings.Cut(r.Header.Get("Authorization"), " ")

		if !found || !strings.EqualFold(scheme, "Bearer") {
			writeJSONError(w, http.StatusUnauthorized, "invalid_token")

			return
		}

		tokenString = strings.TrimSpace(tokenString)

		if tokenString == "" {
			writeJSONError(w, http.StatusUnauthorized, "invalid_token")

			return
		}

		secret := config.GetEnv("JWT_SECRET")

		if secret == "" {
			writeJSONError(w, http.StatusInternalServerError, "internal_error")

			return
		}

		claims := &models.Claims{}

		token, err := jwt.ParseWithClaims(
			tokenString,
			claims,
			func(t *jwt.Token) (any, error) {
				// Only HS256 tokens are ever issued; refuse every
				// other algorithm so a forged alg header cannot pass.
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}

				return []byte(secret), nil
			},
		)

		if err != nil || !token.Valid {
			writeJSONError(w, http.StatusUnauthorized, "invalid_token")

			return
		}

		user := User{
			ID:       claims.UserID,
			Username: claims.Username,
		}

		next.ServeHTTP(w, r.WithContext(
			context.WithValue(r.Context(), userContextKey{}, user),
		))
	})
}

// writeJSONError writes the shared {"error": ...} response body.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	json.NewEncoder(w).Encode(map[string]any{"error": message})
}
