// Package models holds the JSON request and response models shared
// by the HTTP handlers, plus the JWT claims structure.
package models

import "github.com/golang-jwt/jwt/v5"

// RegisterReq is the body of a registration request.
type RegisterReq struct {
	Name     string `json:"name"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginReq is the body of a login request.
type LoginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RefreshReq is the body of a token refresh request.
type RefreshReq struct {
	RefreshToken string `json:"refreshToken"`
}

// Claims is the payload of a Strata access token: the user's id and
// username inside the standard JWT registered claims.
type Claims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`

	jwt.RegisteredClaims
}
