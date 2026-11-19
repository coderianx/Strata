// Package teams implements the team HTTP endpoints.
package teams

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

// CreateNewTeamHandler validates the creation body and stores a
// team inside the caller's organization, answering 201 with the new
// id. The caller must be the organization's owner (404 otherwise)
// and the identifier must be free (409 otherwise).
func CreateNewTeamHandler(
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

	var req models.CreateNewTeamReq

	err = json.NewDecoder(r.Body).Decode(&req)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_request",
		})

		return
	}

	normalizeCreateNewTeamReq(&req)

	err = validateCreateNewTeamReq(req)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": err.Error(),
		})

		return
	}

	teamID, err := repository.CreateNewTeam(
		r.Context(),
		organizationID,
		req.Name,
		req.Identifier,
		req.Description,
		claims.ID,
	)

	if err != nil {

		if errors.Is(err, repository.ErrOrganizationNotFound) {
			w.WriteHeader(404)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "organization_not_found_or_access_denied",
			})

			return
		}

		if errors.Is(err, repository.ErrTeamAlreadyExists) {
			w.WriteHeader(409)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "team_already_exists",
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
		"message": "team_created",
		"teamID":  teamID,
	})
}

// DeleteTeamHandler removes a team from the caller's organization
// and answers 200. The caller must be the organization's owner and
// the team must belong to it; both failures answer 404.
func DeleteTeamHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	w.Header().Set("Content-Type", "application/json")

	claims, ok := middlewares.UserFromContext(
		r.Context(),
	)

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

	teamID, err := strconv.ParseInt(
		chi.URLParam(r, "team_id"), 10, 64,
	)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_team_id",
		})

		return
	}

	if teamID <= 0 {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_team_id",
		})

		return
	}

	err = repository.DeleteTeam(
		r.Context(),
		organizationID,
		teamID,
		claims.ID,
	)

	if err != nil {

		if errors.Is(err, repository.ErrOrganizationNotFound) {
			w.WriteHeader(404)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "organization_not_found_or_access_denied",
			})

			return
		}

		if errors.Is(err, repository.ErrTeamNotFound) {
			w.WriteHeader(404)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "team_not_found_or_access_denied",
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
		"message": "team_deleted",
	})
}
