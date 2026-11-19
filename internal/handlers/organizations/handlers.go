// Package organizations implements the organization, membership and
// invitation HTTP endpoints.
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

// CreateNewOrganizationHandler validates the creation body, stores
// the organization with its owner membership and settings, and
// answers 201. Invalid bodies get 400, taken slugs 409.
func CreateNewOrganizationHandler(
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
		req.Visibility,
		req.ShowMembers,
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

// DeleteOrganization removes the caller's organization and answers
// 200. Missing and not-owned organizations are indistinguishably
// 404.
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

// InviteUserToOrganizationHandler invites an existing user to the
// caller's organization and answers 201. The caller must be the
// organization's owner; missing and not-owned organizations are
// indistinguishably 404, an existing member or a duplicate pending
// invitation gets 409.
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

// ListMyInvitationsHandler returns the caller's pending invitations
// as {"invitations": [...]}.
func ListMyInvitationsHandler(
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

	invitations, err := repository.ListReceivedInvitations(
		r.Context(),
		claims.ID,
	)

	if err != nil {
		w.WriteHeader(500)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "internal_error",
		})

		return
	}

	w.WriteHeader(200)

	json.NewEncoder(w).Encode(map[string]any{
		"invitations": invitations,
	})

}

// AcceptInvitationHandler turns one of the caller's pending
// invitations into a membership and answers 200. Unknown, foreign or
// already-answered invitations are 404; a membership conflict is
// 409.
func AcceptInvitationHandler(
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

	invitationID, err := strconv.ParseInt(
		chi.URLParam(r, "invitation_id"), 10, 64,
	)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_invitation_id",
		})

		return
	}

	if invitationID <= 0 {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_invitation_id",
		})

		return
	}

	err = repository.AcceptInvitation(
		r.Context(),
		claims.ID,
		invitationID,
	)

	if err != nil {
		if errors.Is(err, repository.ErrInvitationNotFound) {
			w.WriteHeader(404)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "invitation_not_found",
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

		w.WriteHeader(500)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "internal_error",
		})

		return
	}

	w.WriteHeader(200)

	json.NewEncoder(w).Encode(map[string]any{
		"message": "invitation_accepted",
	})
}

// RejectInvitationHandler marks one of the caller's pending
// invitations as rejected and answers 200, letting the organization
// invite the same user again later. Unknown or foreign invitations
// are 404.
func RejectInvitationHandler(
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

	invitationID, err := strconv.ParseInt(
		chi.URLParam(r, "invitation_id"), 10, 64,
	)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_invitation_id",
		})

		return
	}

	if invitationID <= 0 {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_invitation_id",
		})

		return
	}

	err = repository.RejectInvitation(
		r.Context(),
		claims.ID,
		invitationID,
	)

	if err != nil {
		if errors.Is(err, repository.ErrInvitationNotFound) {
			w.WriteHeader(404)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "invitation_not_found",
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
		"message": "invitation_rejected",
	})
}

// GetMyOrganizationsHandler returns the organizations the caller is
// a member of as {"organizations": [...]}.
func GetMyOrganizationsHandler(
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

	organizations, err := repository.GetMyOrganizations(
		r.Context(),
		claims.ID,
	)

	if err != nil {
		w.WriteHeader(500)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "internal_error",
		})

		return
	}

	w.WriteHeader(200)

	json.NewEncoder(w).Encode(map[string]any{
		"organizations": organizations,
	})
}

// OrganizationMembersHandler returns an organization's member
// list. Members always see it; outsiders get an indistinguishable
// 404 on private organizations and 403 on public ones with hidden
// member lists.
func OrganizationMembersHandler(
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

	members, err := repository.GetOrganizationMembers(
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

		if errors.Is(err, repository.ErrOrganizationMembersIsConfidential) {
			w.WriteHeader(403)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "members_confidential",
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
		"members": members,
	})
}

// RemoveMemberHandler removes a member from the caller's
// organization. The caller must be its owner (404 otherwise) and
// cannot remove themselves (409).
func RemoveMemberHandler(
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

	memberID, err := strconv.ParseInt(
		chi.URLParam(r, "member_id"), 10, 64,
	)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_member_id",
		})

		return
	}

	if memberID <= 0 {
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_member_id",
		})

		return
	}

	err = repository.RemoveMember(
		r.Context(),
		organizationID,
		claims.ID,
		memberID,
	)

	if err != nil {
		if errors.Is(err, repository.ErrOrganizationNotFound) {
			w.WriteHeader(404)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "organization_not_found",
			})

			return
		}

		if errors.Is(err, repository.ErrCannotRemoveOwner) {
			w.WriteHeader(409)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "cannot_remove_owner",
			})

			return
		}

		if errors.Is(err, repository.ErrMemberNotFound) {
			w.WriteHeader(404)

			json.NewEncoder(w).Encode(map[string]any{
				"error": "member_not_found",
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
		"message": "member_removed",
	})
}
