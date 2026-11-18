package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coderianx/Strata/internal/config"
	"github.com/coderianx/Strata/internal/models"
	"github.com/golang-jwt/jwt/v5"
)

const (
	minUsernameLength = 3
	maxUsernameLength = 20

	minPasswordLength = 8
	// bcrypt only hashes the first 72 bytes of a password.
	maxPasswordLength = 72

	// accessTokenTTL is intentionally short: clients exchange their
	// refresh token for a new one when it expires.
	accessTokenTTL = 15 * time.Minute

	// refreshTokenTTL is how long a refresh token stays valid in the
	// refresh_tokens table.
	refreshTokenTTL = 30 * 24 * time.Hour
)

var usernameRe = regexp.MustCompile(`^[a-z0-9_]+$`)

// reservedUsernames cannot be registered by users. Extend as needed.
var reservedUsernames = map[string]struct{}{
	"admin":         {},
	"administrator": {},
	"anonymous":     {},
	"api":           {},
	"auth":          {},
	"billing":       {},
	"help":          {},
	"hostmaster":    {},
	"info":          {},
	"login":         {},
	"logout":        {},
	"mail":          {},
	"manager":       {},
	"mod":           {},
	"moderator":     {},
	"null":          {},
	"password":      {},
	"postmaster":    {},
	"profile":       {},
	"register":      {},
	"root":          {},
	"security":      {},
	"settings":      {},
	"staff":         {},
	"strata":        {},
	"support":       {},
	"system":        {},
	"undefined":     {},
	"webmaster":     {},
	"www":           {},
}

// validateUsername enforces the common username rules used by mainstream
// apps (Telegram/GitHub style):
//   - 3-20 characters,
//   - only lowercase letters, numbers and underscores,
//   - must start with a letter,
//   - cannot end with an underscore,
//   - no consecutive underscores,
//   - cannot be a reserved name (admin, root, api, ...).
func validateUsername(username string) error {
	if len(username) < minUsernameLength {
		return fmt.Errorf("username must be at least %d characters", minUsernameLength)
	}

	if len(username) > maxUsernameLength {
		return fmt.Errorf("username must be at most %d characters", maxUsernameLength)
	}

	if !usernameRe.MatchString(username) {
		return errors.New("username can only contain lowercase letters, numbers and underscores")
	}

	if username[0] < 'a' || username[0] > 'z' {
		return errors.New("username must start with a letter")
	}

	if username[len(username)-1] == '_' {
		return errors.New("username cannot end with an underscore")
	}

	if strings.Contains(username, "__") {
		return errors.New("username cannot contain consecutive underscores")
	}

	if _, ok := reservedUsernames[username]; ok {
		return errors.New("this username is reserved")
	}

	return nil
}

// normalizeRegisterReq trims the text fields and lowercases the email so
// the exact values that pass validation are the ones stored. The password
// is deliberately left untouched: leading/trailing spaces can be part of
// a password.
func normalizeRegisterReq(req *models.RegisterReq) {
	req.Name = strings.TrimSpace(req.Name)
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
}

func validateRegisterReq(req models.RegisterReq) error {
	if req.Name == "" {
		return errors.New("name is required")
	}

	if err := validateUsername(req.Username); err != nil {
		return err
	}

	at := strings.Index(req.Email, "@")

	if at <= 0 || !strings.Contains(req.Email[at+1:], ".") {
		return errors.New("email is invalid")
	}

	if len(req.Password) < minPasswordLength {
		return fmt.Errorf("password must be at least %d characters", minPasswordLength)
	}

	if len(req.Password) > maxPasswordLength {
		return fmt.Errorf("password must be at most %d bytes", maxPasswordLength)
	}

	return nil
}

// normalizeLoginReq lowercases the email exactly like normalizeRegisterReq
// so login lookups match the values stored at registration. The password is
// deliberately left untouched.
func normalizeLoginReq(req *models.LoginReq) {
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
}

// validateLoginReq only checks that the fields are present. Register rules
// (password length etc.) must not block users whose accounts were created
// under older rules; wrong credentials fail at the bcrypt compare anyway.
func validateLoginReq(req models.LoginReq) error {
	if req.Email == "" || req.Password == "" {
		return errors.New("email and password are required")
	}

	return nil
}

// generateJWT signs a short-lived HS256 access token carrying the user's
// id and username.
func generateJWT(userID int64, username string) (string, error) {
	secret := config.GetEnv("JWT_SECRET")

	if len(secret) == 0 {
		return "", errors.New("auth: JWT_SECRET is not set")
	}

	now := time.Now()

	claims := models.Claims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(accessTokenTTL)),
			Issuer:    "strata",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString([]byte(secret))
}

// generateRefreshToken returns a random 64-character hex token. Only its
// SHA-256 hash is stored, so a leaked database cannot be replayed against
// the API; the plaintext exists only in the login response.
func generateRefreshToken() (string, error) {
	buf := make([]byte, 32)

	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("auth: generate refresh token: %w", err)
	}

	return hex.EncodeToString(buf), nil
}

// hashRefreshToken returns the SHA-256 hex digest of a refresh token,
// matching the CHAR(64) token_hash column.
func hashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))

	return hex.EncodeToString(sum[:])
}

// normalizeRefreshReq trims the token: issued tokens are pure hex and
// never contain whitespace, so padding around them is always client noise.
func normalizeRefreshReq(req *models.RefreshReq) {
	req.RefreshToken = strings.TrimSpace(req.RefreshToken)
}

func validateRefreshReq(req models.RefreshReq) error {
	if req.RefreshToken == "" {
		return errors.New("refreshToken is required")
	}

	return nil
}
