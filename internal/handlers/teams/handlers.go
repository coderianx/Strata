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

// AddNewMemberToTeamHandler adds an existing user to a team with the
// requested role. The caller must be a creator or admin of the team
// (403 otherwise) and the 'creator' role cannot be assigned. The user
// being added must already be a member of the organization (409
// otherwise). It answers 201 on success, 404 for an unknown user and
// 409 when the user is already a team member.
func AddNewMemberToTeamHandler(
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

	if err != nil || organizationID <= 0 {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_organization_id",
		})

		return
	}

	teamID, err := strconv.ParseInt(
		chi.URLParam(r, "team_id"), 10, 64,
	)

	if err != nil || teamID <= 0 {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_team_id",
		})

		return
	}

	var req models.AddNewMemberToTeamReq

	err = json.NewDecoder(r.Body).Decode(&req)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_request",
		})

		return
	}

	normalizeAddNewMemberToTeamReq(&req)

	err = validateAddNewMemberToTeamReq(req)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": err.Error(),
		})

		return
	}

	newMemberID, err := repository.GetUserByUsername(
		r.Context(),
		req.Username,
	)

	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			w.WriteHeader(http.StatusNotFound)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "user_not_found",
			})

			return
		}

		w.WriteHeader(http.StatusInternalServerError)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "internal_error",
		})

		return
	}

	err = repository.AddNewMemberToTeam(
		r.Context(),
		teamID,
		newMemberID,
		req.Role,
		claims.ID,
	)

	if err != nil {
		if errors.Is(err, repository.ErrTeamAccessDenied) {
			w.WriteHeader(http.StatusForbidden)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "team_access_denied",
			})

			return
		}

		if errors.Is(err, repository.ErrUserNotOrganizationMember) {
			w.WriteHeader(http.StatusConflict)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "user_not_organization_member",
			})

			return
		}

		if errors.Is(err, repository.ErrUserAlreadyMember) {
			w.WriteHeader(http.StatusConflict)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "user_already_member",
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
		"message": "team_member_added",
	})
}

// ListOrganizationTeamsHandler returns the teams of the given
// organization as {"teams": [...]}. The caller must be a member of
// the organization; non-members and missing organizations are
// indistinguishably 404.
func ListOrganizationTeamsHandler(
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

	if err != nil || organizationID <= 0 {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_organization_id",
		})

		return
	}

	teams, err := repository.ListOrganizationTeams(
		r.Context(),
		organizationID,
		claims.ID,
	)

	if err != nil {
		if errors.Is(err, repository.ErrOrganizationNotFound) {
			w.WriteHeader(http.StatusNotFound)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "organization_not_found",
			})

			return
		}

		w.WriteHeader(http.StatusInternalServerError)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "internal_error",
		})

		return
	}

	w.WriteHeader(http.StatusOK)

	json.NewEncoder(w).Encode(map[string]any{
		"teams": teams,
	})
}

// RemoveMemberFromTeamHandler removes a member from a team. The
// caller must be a creator or admin of the team (403 otherwise); the
// team's creator cannot be removed by anyone, including themselves
// (409). It answers 200 on success and 404 when the member is not
// part of the team.
func RemoveMemberFromTeamHandler(
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

	if err != nil || organizationID <= 0 {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_organization_id",
		})

		return
	}

	teamID, err := strconv.ParseInt(
		chi.URLParam(r, "team_id"), 10, 64,
	)

	if err != nil || teamID <= 0 {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_team_id",
		})

		return
	}

	memberID, err := strconv.ParseInt(
		chi.URLParam(r, "member_id"), 10, 64,
	)

	if err != nil || memberID <= 0 {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_member_id",
		})

		return
	}

	err = repository.RemoveMemberFromTeam(
		r.Context(),
		teamID,
		memberID,
		claims.ID,
	)

	if err != nil {
		if errors.Is(err, repository.ErrTeamAccessDenied) {
			w.WriteHeader(http.StatusForbidden)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "team_access_denied",
			})

			return
		}

		if errors.Is(err, repository.ErrCannotRemoveCreator) {
			w.WriteHeader(http.StatusConflict)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "cannot_remove_creator",
			})

			return
		}

		if errors.Is(err, repository.ErrMemberNotFound) {
			w.WriteHeader(http.StatusNotFound)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "member_not_found",
			})

			return
		}

		w.WriteHeader(http.StatusInternalServerError)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "internal_error",
		})

		return
	}

	w.WriteHeader(http.StatusOK)

	json.NewEncoder(w).Encode(map[string]any{
		"message": "team_member_removed",
	})
}
