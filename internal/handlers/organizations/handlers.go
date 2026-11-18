package organizations

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/coderianx/Strata/internal/middlewares"
	"github.com/coderianx/Strata/internal/models"
	"github.com/coderianx/Strata/internal/repository"
	"github.com/go-chi/chi/v5"
)

func CreateNewOrganization(
	w http.ResponseWriter,
	r *http.Request,
) {
	w.Header().Set("Content-Type", "application/json")

	claims, ok := middlewares.UserFromContext(r.Context())

	if !ok {
		w.WriteHeader(http.StatusUnauthorized)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "unauthorized",
		})

		return
	}

	var req models.CreateNewOrganizationReq

	err := json.NewDecoder(r.Body).Decode(&req)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_request",
		})

		return
	}

	normalizeCreateNewOrganizationReq(&req)

	err = validateCreateNewOrganizationReq(req)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": err.Error(),
		})

		return
	}

	err = repository.CreateNewOrganization(
		r.Context(),
		req.Name,
		req.Slug,
		req.Description,
		req.LogoURL,
		claims.ID,
	)

	if err != nil {
		if errors.Is(err, repository.ErrOrganizationAlreadyExists) {
			w.WriteHeader(409)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "organization_already_exists",
			})

			return
		}

		w.WriteHeader(500)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "internal_error",
		})

		return
	}

	w.WriteHeader(201)

	json.NewEncoder(w).Encode(map[string]any{
		"message": "organization_created",
	})
}

func DeleteOrganization(
	w http.ResponseWriter,
	r *http.Request,
) {
	w.Header().Set("Content-Type", "application/json")

	claims, ok := middlewares.UserFromContext(r.Context())

	if !ok {
		w.WriteHeader(http.StatusUnauthorized)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "unauthorized",
		})

		return
	}

	organizationID, err := strconv.ParseInt(
		chi.URLParam(r, "organization_id"), 10, 64,
	)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_organization_id",
		})

		return
	}

	if organizationID <= 0 {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_organization_id",
		})

		return
	}

	err = repository.DeleteOrganization(
		r.Context(),
		organizationID,
		claims.ID,
	)

	if err != nil {
		if errors.Is(err, repository.ErrOrganizationNotFound) {
			w.WriteHeader(404)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "organization_not_found",
			})

			return
		}

		w.WriteHeader(500)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "internal_error",
		})

		return
	}

	w.WriteHeader(200)

	json.NewEncoder(w).Encode(map[string]any{
		"message": "organization_deleted",
	})
}

func InviteUserToOrganizationHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	w.Header().Set("Content-Type", "application/json")

	claims, ok := middlewares.UserFromContext(r.Context())

	if !ok {
		w.WriteHeader(http.StatusUnauthorized)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "unauthorized",
		})

		return
	}

	organizationID, err := strconv.ParseInt(
		chi.URLParam(r, "organization_id"), 10, 64,
	)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_organization_id",
		})

		return
	}

	if organizationID <= 0 {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_organization_id",
		})

		return
	}

	var req models.InviteSomeoneToOrganizationReq

	err = json.NewDecoder(r.Body).Decode(&req)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_request",
		})

		return
	}

	normalizeInviteSomeoneToOrganizationReq(&req)

	err = validateInviteSomeoneToOrganizationReq(req)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": err.Error(),
		})

		return
	}

	err = repository.InviteOrganization(
		r.Context(),
		organizationID,
		claims.ID,
		req.Username,
		req.Role,
	)

	if err != nil {
		if errors.Is(err, repository.ErrOrganizationNotFound) {
			w.WriteHeader(404)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "organization_not_found",
			})

			return
		}

		if errors.Is(err, repository.ErrUserNotFound) {
			w.WriteHeader(404)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "user_not_found",
			})

			return
		}

		if errors.Is(err, repository.ErrUserAlreadyMember) {
			w.WriteHeader(409)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "user_already_member",
			})

			return
		}

		if errors.Is(err, repository.ErrInvitationAlreadyExists) {
			w.WriteHeader(409)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "invitation_already_exists",
			})

			return
		}

		w.WriteHeader(500)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "internal_error",
		})

		return
	}

	w.WriteHeader(201)

	json.NewEncoder(w).Encode(map[string]any{
		"message": "user_invited",
	})
}
