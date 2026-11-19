// Package auth implements the registration, login and token refresh
// HTTP endpoints.
package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/coderianx/Strata/internal/middlewares"
	"github.com/coderianx/Strata/internal/models"
	"github.com/coderianx/Strata/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

// RegisterHandler validates the registration body, hashes the
// password with bcrypt and stores the user. It answers 201 with the
// created user, 400 for invalid or incomplete bodies and 409 when
// the username or email is already taken.
func RegisterHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	w.Header().Set("Content-Type", "application/json")

	var req models.RegisterReq

	err := json.NewDecoder(r.Body).Decode(&req)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_request",
		})

		return
	}

	normalizeRegisterReq(&req)

	err = validateRegisterReq(req)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": err.Error(),
		})

		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword(
		[]byte(req.Password),
		bcrypt.DefaultCost,
	)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "internal_error",
		})

		return
	}

	userID, err := repository.InsertNewUser(
		r.Context(),
		req.Name,
		req.Username,
		req.Email,
		string(hashedPassword),
	)

	if err != nil {
		if errors.Is(err, repository.ErrDuplicateUser) {
			w.WriteHeader(http.StatusConflict)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "username or email is already taken",
			})

			return
		}

		w.WriteHeader(http.StatusInternalServerError)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "internal_error",
		})

		return
	}

	w.WriteHeader(http.StatusCreated)

	json.NewEncoder(w).Encode(map[string]any{
		"id":       userID,
		"name":     req.Name,
		"username": req.Username,
		"email":    req.Email,
	})
}

// LoginHandler verifies the login credentials against the stored
// bcrypt hash and issues an access/refresh token pair. It answers
// 200 with the tokens, 400 for invalid bodies and 401 for wrong
// credentials.
func LoginHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	w.Header().Set("Content-Type", "application/json")

	var req models.LoginReq

	err := json.NewDecoder(r.Body).Decode(&req)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_request",
		})

		return
	}

	normalizeLoginReq(&req)

	err = validateLoginReq(req)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": err.Error(),
		})

		return
	}

	userID, username, passwordHash, err := repository.GetUserByEmail(
		r.Context(),
		req.Email,
	)

	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			w.WriteHeader(http.StatusUnauthorized)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "invalid_credentials",
			})

			return
		}

		w.WriteHeader(http.StatusInternalServerError)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "internal_error",
		})

		return
	}

	err = bcrypt.CompareHashAndPassword(
		[]byte(passwordHash),
		[]byte(req.Password),
	)

	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_credentials",
		})

		return
	}

	accessToken, err := generateJWT(userID, username)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "internal_error",
		})

		return
	}

	refreshToken, err := generateRefreshToken()

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "internal_error",
		})

		return
	}

	err = repository.InsertRefreshToken(
		r.Context(),
		userID,
		hashRefreshToken(refreshToken),
		time.Now().Add(refreshTokenTTL),
	)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "internal_error",
		})

		return
	}

	w.WriteHeader(http.StatusOK)

	json.NewEncoder(w).Encode(map[string]any{
		"accessToken":  accessToken,
		"refreshToken": refreshToken,
	})
}

// RefreshHandler consumes a single-use refresh token and issues a
// new access/refresh token pair. Unknown, expired or replayed
// tokens get 401; invalid bodies 400.
func RefreshHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	w.Header().Set("Content-Type", "application/json")

	var req models.RefreshReq

	err := json.NewDecoder(r.Body).Decode(&req)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_request",
		})

		return
	}

	normalizeRefreshReq(&req)

	err = validateRefreshReq(req)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": err.Error(),
		})

		return
	}

	userID, err := repository.ConsumeRefreshToken(
		r.Context(),
		hashRefreshToken(req.RefreshToken),
	)

	if err != nil {
		if errors.Is(err, repository.ErrRefreshTokenInvalid) {
			w.WriteHeader(http.StatusUnauthorized)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "invalid_refresh_token",
			})

			return
		}

		w.WriteHeader(http.StatusInternalServerError)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "internal_error",
		})

		return
	}

	username, err := repository.GetUsernameByID(r.Context(), userID)

	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			w.WriteHeader(http.StatusUnauthorized)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "invalid_refresh_token",
			})

			return
		}

		w.WriteHeader(http.StatusInternalServerError)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "internal_error",
		})

		return
	}

	accessToken, err := generateJWT(userID, username)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "internal_error",
		})

		return
	}

	refreshToken, err := generateRefreshToken()

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "internal_error",
		})

		return
	}

	err = repository.InsertRefreshToken(
		r.Context(),
		userID,
		hashRefreshToken(refreshToken),
		time.Now().Add(refreshTokenTTL),
	)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "internal_error",
		})

		return
	}

	w.WriteHeader(http.StatusOK)

	json.NewEncoder(w).Encode(map[string]any{
		"accessToken":  accessToken,
		"refreshToken": refreshToken,
	})
}

// GetMeHandler returns the authenticated user's id and username. It
// must be mounted behind middlewares.Auth; requests that skipped the
// middleware get the shared 401 invalid_token response.
func GetMeHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	w.Header().Set("Content-Type", "application/json")

	user, ok := middlewares.UserFromContext(r.Context())

	if !ok {
		w.WriteHeader(http.StatusUnauthorized)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_token",
		})

		return
	}

	w.WriteHeader(http.StatusOK)

	json.NewEncoder(w).Encode(map[string]any{
		"id":       user.ID,
		"username": user.Username,
	})
}
