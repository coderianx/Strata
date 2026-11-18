//go:build integration

// End-to-end tests for the auth handlers backed by a real PostgreSQL
// instance. Run them with:
//
//	TEST_DATABASE_URL=postgres://user:pass@localhost/strata_test \
//		go test -tags=integration ./...
//
// The database must be disposable: every test truncates the users and
// refresh_tokens tables.

package auth

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/coderianx/Strata/internal/models"
	"github.com/coderianx/Strata/internal/repository"
	"github.com/coderianx/Strata/internal/store"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	testDBEnv  = "TEST_DATABASE_URL"
	testSecret = "integration-test-secret"
)

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
		`TRUNCATE users, organizations, organization_members, organization_invitations, refresh_tokens RESTART IDENTITY CASCADE`,
	)
	if err != nil {
		t.Fatalf("reset tables: %v", err)
	}
}

// insertUser stores a fixture user directly through the repository,
// bypassing the handlers. The bcrypt hash uses the minimum cost:
// fixture strength is irrelevant, test speed is not.
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

// login calls the login handler with the given credentials.
func login(t *testing.T, email, password string) *httptest.ResponseRecorder {
	t.Helper()

	body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)

	return callHandler(t, LoginHandler, body)
}

// parseAccessToken verifies the token's signature with the test
// secret and returns its claims.
func parseAccessToken(t *testing.T, tokenString string) models.Claims {
	t.Helper()

	claims := &models.Claims{}

	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}

			return []byte(testSecret), nil
		},
	)
	if err != nil || !token.Valid {
		t.Fatalf("accessToken is not a valid signed token: %v", err)
	}

	return *claims
}

// assertAccessToken checks the access token's claims and that it
// lives no longer than accessTokenTTL.
func assertAccessToken(t *testing.T, tokenString string, userID int64, username string) {
	t.Helper()

	now := time.Now()

	claims := parseAccessToken(t, tokenString)

	if claims.UserID != userID {
		t.Errorf("claims.UserID = %d, want %d", claims.UserID, userID)
	}

	if claims.Username != username {
		t.Errorf("claims.Username = %q, want %q", claims.Username, username)
	}

	if claims.Subject != strconv.FormatInt(userID, 10) {
		t.Errorf("claims.Subject = %q, want %q", claims.Subject, strconv.FormatInt(userID, 10))
	}

	if claims.Issuer != "strata" {
		t.Errorf("claims.Issuer = %q, want %q", claims.Issuer, "strata")
	}

	if claims.ExpiresAt == nil {
		t.Fatal("claims.ExpiresAt is not set")
	}

	ttl := claims.ExpiresAt.Time.Sub(now)
	if ttl <= 0 {
		t.Errorf("access token already expired at %v", claims.ExpiresAt.Time)
	}

	if ttl > accessTokenTTL {
		t.Errorf("access token TTL = %v, want at most %v", ttl, accessTokenTTL)
	}
}

// assertRefreshToken checks that a refresh token is 64 hex chars,
// the format issued by generateRefreshToken.
func assertRefreshToken(t *testing.T, token string) {
	t.Helper()

	if len(token) != 64 {
		t.Fatalf("refreshToken length = %d, want 64", len(token))
	}

	if _, err := hex.DecodeString(token); err != nil {
		t.Errorf("refreshToken is not hex: %v", err)
	}
}

func TestRegisterHandlerIntegration(t *testing.T) {
	requireDB(t)

	// Padding and mixed case on purpose: the handler must normalize
	// before storing and echo back the normalized values.
	body := `{"name":"  Kodcu ","username":"  kodcu_42  ","email":"  Kodcu@Strata.DEV ","password":"sifre1234"}`

	rec := callHandler(t, RegisterHandler, body)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s",
			rec.Code, http.StatusCreated, rec.Body.String())
	}

	var res struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	decodeResponse(t, rec, &res)

	if res.ID <= 0 {
		t.Errorf("id = %d, want a positive id", res.ID)
	}

	if res.Name != "Kodcu" {
		t.Errorf("name = %q, want %q", res.Name, "Kodcu")
	}

	if res.Username != "kodcu_42" {
		t.Errorf("username = %q, want %q", res.Username, "kodcu_42")
	}

	if res.Email != "kodcu@strata.dev" {
		t.Errorf("email = %q, want %q", res.Email, "kodcu@strata.dev")
	}

	// The stored row must match the response and hold a password hash
	// that still verifies the plaintext.
	userID, username, hash, err := repository.GetUserByEmail(
		context.Background(),
		"kodcu@strata.dev",
	)
	if err != nil {
		t.Fatalf("GetUserByEmail() = %v, want nil", err)
	}

	if userID != res.ID {
		t.Errorf("stored id = %d, want %d", userID, res.ID)
	}

	if username != res.Username {
		t.Errorf("stored username = %q, want %q", username, res.Username)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("sifre1234")); err != nil {
		t.Errorf("stored password hash does not verify: %v", err)
	}
}

func TestRegisterHandlerDuplicateIntegration(t *testing.T) {
	requireDB(t)

	insertUser(t, "kodcu_42", "kodcu@strata.dev")

	tests := []struct {
		name string
		body string
	}{
		{
			"duplicate username",
			`{"name":"Kodcu","username":"kodcu_42","email":"other@strata.dev","password":"sifre1234"}`,
		},
		{
			"duplicate email",
			`{"name":"Kodcu","username":"other_user","email":"kodcu@strata.dev","password":"sifre1234"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := callHandler(t, RegisterHandler, tt.body)

			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, want %d; body = %s",
					rec.Code, http.StatusConflict, rec.Body.String())
			}

			var res struct {
				Error string `json:"error"`
			}
			decodeResponse(t, rec, &res)

			want := "username or email is already taken"
			if res.Error != want {
				t.Errorf("error = %q, want %q", res.Error, want)
			}
		})
	}
}

func TestLoginHandlerIntegration(t *testing.T) {
	requireDB(t)

	userID := insertUser(t, "kodcu_42", "kodcu@strata.dev")

	// Padding and mixed case on purpose: login must normalize the
	// email the same way registration does.
	rec := login(t, "  Kodcu@Strata.DEV ", "sifre1234")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s",
			rec.Code, http.StatusOK, rec.Body.String())
	}

	var res struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	decodeResponse(t, rec, &res)

	assertAccessToken(t, res.AccessToken, userID, "kodcu_42")
	assertRefreshToken(t, res.RefreshToken)

	// Only the SHA-256 hash of the refresh token is stored, and it
	// must map back to the user who logged in.
	got, err := repository.ConsumeRefreshToken(
		context.Background(),
		hashRefreshToken(res.RefreshToken),
	)
	if err != nil {
		t.Fatalf("refresh token was not stored: %v", err)
	}

	if got != userID {
		t.Errorf("refresh token user = %d, want %d", got, userID)
	}
}

func TestLoginHandlerInvalidCredentialsIntegration(t *testing.T) {
	requireDB(t)

	insertUser(t, "kodcu_42", "kodcu@strata.dev")

	tests := []struct {
		name     string
		email    string
		password string
	}{
		{"wrong password", "kodcu@strata.dev", "wrong-sifre"},
		{"unknown email", "nobody@strata.dev", "sifre1234"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := login(t, tt.email, tt.password)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d; body = %s",
					rec.Code, http.StatusUnauthorized, rec.Body.String())
			}

			var res struct {
				Error string `json:"error"`
			}
			decodeResponse(t, rec, &res)

			if res.Error != "invalid_credentials" {
				t.Errorf("error = %q, want %q", res.Error, "invalid_credentials")
			}
		})
	}
}

func TestRefreshHandlerIntegration(t *testing.T) {
	requireDB(t)

	userID := insertUser(t, "kodcu_42", "kodcu@strata.dev")

	oldToken := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := repository.InsertRefreshToken(
		context.Background(),
		userID,
		hashRefreshToken(oldToken),
		time.Now().Add(time.Hour),
	); err != nil {
		t.Fatalf("insert fixture token: %v", err)
	}

	rec := callHandler(
		t,
		RefreshHandler,
		fmt.Sprintf(`{"refreshToken":%q}`, oldToken),
	)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s",
			rec.Code, http.StatusOK, rec.Body.String())
	}

	var res struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	decodeResponse(t, rec, &res)

	assertAccessToken(t, res.AccessToken, userID, "kodcu_42")
	assertRefreshToken(t, res.RefreshToken)

	if res.RefreshToken == oldToken {
		t.Error("refresh handler must rotate the refresh token")
	}

	// The old token was consumed and can never be replayed.
	_, err := repository.ConsumeRefreshToken(
		context.Background(),
		hashRefreshToken(oldToken),
	)
	if !errors.Is(err, repository.ErrRefreshTokenInvalid) {
		t.Errorf("old token still usable: %v, want ErrRefreshTokenInvalid", err)
	}

	// The new token is stored and maps back to the same user.
	got, err := repository.ConsumeRefreshToken(
		context.Background(),
		hashRefreshToken(res.RefreshToken),
	)
	if err != nil {
		t.Fatalf("new refresh token was not stored: %v", err)
	}

	if got != userID {
		t.Errorf("new token user = %d, want %d", got, userID)
	}
}

func TestRefreshHandlerReuseIntegration(t *testing.T) {
	requireDB(t)

	userID := insertUser(t, "kodcu_42", "kodcu@strata.dev")

	t.Run("a consumed token is rejected", func(t *testing.T) {
		token := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

		if err := repository.InsertRefreshToken(
			context.Background(),
			userID,
			hashRefreshToken(token),
			time.Now().Add(time.Hour),
		); err != nil {
			t.Fatalf("insert fixture token: %v", err)
		}

		first := callHandler(
			t,
			RefreshHandler,
			fmt.Sprintf(`{"refreshToken":%q}`, token),
		)
		if first.Code != http.StatusOK {
			t.Fatalf("first refresh status = %d, want %d; body = %s",
				first.Code, http.StatusOK, first.Body.String())
		}

		second := callHandler(
			t,
			RefreshHandler,
			fmt.Sprintf(`{"refreshToken":%q}`, token),
		)
		if second.Code != http.StatusUnauthorized {
			t.Fatalf("reused token status = %d, want %d; body = %s",
				second.Code, http.StatusUnauthorized, second.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, second, &res)

		if res.Error != "invalid_refresh_token" {
			t.Errorf("error = %q, want %q", res.Error, "invalid_refresh_token")
		}
	})

	t.Run("an unknown token is rejected", func(t *testing.T) {
		token := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

		rec := callHandler(
			t,
			RefreshHandler,
			fmt.Sprintf(`{"refreshToken":%q}`, token),
		)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d; body = %s",
				rec.Code, http.StatusUnauthorized, rec.Body.String())
		}

		var res struct {
			Error string `json:"error"`
		}
		decodeResponse(t, rec, &res)

		if res.Error != "invalid_refresh_token" {
			t.Errorf("error = %q, want %q", res.Error, "invalid_refresh_token")
		}
	})
}
