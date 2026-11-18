package auth

import (
	"fmt"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/coderianx/Strata/internal/models"
)

func TestNormalizeRegisterReq(t *testing.T) {
	req := models.RegisterReq{
		Name:     "  Kodcu  ",
		Username: "  ahmet_42  ",
		Email:    "  Ahmet@Example.COM ",
		Password: "  sifre123  ",
	}

	normalizeRegisterReq(&req)

	if req.Name != "Kodcu" {
		t.Errorf("name = %q, want %q", req.Name, "Kodcu")
	}

	if req.Username != "ahmet_42" {
		t.Errorf("username = %q, want %q", req.Username, "ahmet_42")
	}

	if req.Email != "ahmet@example.com" {
		t.Errorf("email = %q, want %q", req.Email, "ahmet@example.com")
	}

	if req.Password != "  sifre123  " {
		t.Errorf("password must stay untouched, got %q", req.Password)
	}
}

func TestNormalizeThenValidateRegisterReq(t *testing.T) {
	req := models.RegisterReq{
		Name:     " Kodcu ",
		Username: "  ahmet_42  ",
		Email:    "  AHMET@Example.COM ",
		Password: "sifre1234",
	}

	normalizeRegisterReq(&req)

	if err := validateRegisterReq(req); err != nil {
		t.Errorf("expected request to be valid after normalize, got: %v", err)
	}
}

func TestNormalizeLoginReq(t *testing.T) {
	req := models.LoginReq{
		Email:    "  Kodcu@Strata.DEV ",
		Password: "  sifre 123 ",
	}

	normalizeLoginReq(&req)

	if req.Email != "kodcu@strata.dev" {
		t.Errorf("email = %q, want %q", req.Email, "kodcu@strata.dev")
	}

	if req.Password != "  sifre 123 " {
		t.Errorf("password must stay untouched, got %q", req.Password)
	}
}

func TestValidateLoginReq(t *testing.T) {
	cases := []struct {
		name    string
		req     models.LoginReq
		wantErr bool
	}{
		{"valid", models.LoginReq{Email: "kodcu@strata.dev", Password: "sifre1234"}, false},
		{"missing email", models.LoginReq{Password: "sifre1234"}, true},
		{"missing password", models.LoginReq{Email: "kodcu@strata.dev"}, true},
		{"missing both", models.LoginReq{}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateLoginReq(c.req)
			if (err != nil) != c.wantErr {
				t.Errorf("validateLoginReq(%+v) = %v, wantErr %v", c.req, err, c.wantErr)
			}
		})
	}
}

func TestGenerateJWT(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")

	tokenString, err := generateJWT(42, "kodcu")

	if err != nil {
		t.Fatalf("generateJWT returned error: %v", err)
	}

	claims := &models.Claims{}

	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}

			return []byte("test-secret"), nil
		},
	)

	if err != nil || !token.Valid {
		t.Fatalf("generated token is not valid: %v", err)
	}

	if claims.UserID != 42 {
		t.Errorf("claims.UserID = %d, want 42", claims.UserID)
	}

	if claims.Username != "kodcu" {
		t.Errorf("claims.Username = %q, want %q", claims.Username, "kodcu")
	}

	if claims.Subject != "42" {
		t.Errorf("claims.Subject = %q, want %q", claims.Subject, "42")
	}

	if claims.ExpiresAt == nil || !claims.ExpiresAt.After(time.Now()) {
		t.Error("claims.ExpiresAt must be set and in the future")
	}
}

func TestGenerateJWTMissingSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "")

	if _, err := generateJWT(1, "kodcu"); err == nil {
		t.Error("expected an error when JWT_SECRET is not set")
	}
}

func TestGenerateRefreshToken(t *testing.T) {
	tokenA, err := generateRefreshToken()

	if err != nil {
		t.Fatalf("generateRefreshToken returned error: %v", err)
	}

	if len(tokenA) != 64 {
		t.Errorf("token length = %d, want 64", len(tokenA))
	}

	tokenB, err := generateRefreshToken()

	if err != nil {
		t.Fatalf("generateRefreshToken returned error: %v", err)
	}

	if tokenA == tokenB {
		t.Error("two generated tokens must be different")
	}
}

func TestHashRefreshToken(t *testing.T) {
	// sha256("test") — known digest vector.
	want := "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"

	got := hashRefreshToken("test")

	if got != want {
		t.Errorf("hashRefreshToken(%q) = %q, want %q", "test", got, want)
	}

	if len(got) != 64 {
		t.Errorf("digest length = %d, want 64", len(got))
	}
}

func TestNormalizeRefreshReq(t *testing.T) {
	req := models.RefreshReq{RefreshToken: "  f3a1b2c4  "}

	normalizeRefreshReq(&req)

	if req.RefreshToken != "f3a1b2c4" {
		t.Errorf("refreshToken = %q, want %q", req.RefreshToken, "f3a1b2c4")
	}
}

func TestValidateRefreshReq(t *testing.T) {
	cases := []struct {
		name    string
		req     models.RefreshReq
		wantErr bool
	}{
		{"valid", models.RefreshReq{RefreshToken: "f3a1b2c4"}, false},
		{"whitespace only", models.RefreshReq{RefreshToken: "   "}, true},
		{"missing", models.RefreshReq{}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			normalizeRefreshReq(&c.req)

			err := validateRefreshReq(c.req)
			if (err != nil) != c.wantErr {
				t.Errorf("validateRefreshReq(%+v) = %v, wantErr %v", c.req, err, c.wantErr)
			}
		})
	}
}

func TestValidateUsername(t *testing.T) {
	cases := []struct {
		name     string
		username string
		wantErr  bool
	}{
		{"valid simple", "kodcu", false},
		{"valid with digits/underscore", "ahmet_42", false},
		{"valid single underscore", "a_b_c", false},
		{"uppercase", "Kodcu", true},
		{"starts with digit", "1kodcu", true},
		{"starts with underscore", "_kodcu", true},
		{"ends with underscore", "kodcu_", true},
		{"consecutive underscores", "kod__cu", true},
		{"turkish char", "kodçu", true},
		{"space", "kod cu", true},
		{"too short", "ab", true},
		{"too long", "abcdefghijklmnopqrstuvwxyz", true},
		{"empty", "", true},
		{"reserved admin", "admin", true},
		{"reserved moderator", "moderator", true},
		{"reserved only exact match", "kodcu_admin", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateUsername(c.username)
			if (err != nil) != c.wantErr {
				t.Errorf("validateUsername(%q) = %v, wantErr %v", c.username, err, c.wantErr)
			}
		})
	}
}
