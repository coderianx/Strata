// Package handlers builds the top-level HTTP router with every
// route group mounted under it.
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/coderianx/Strata/internal/handlers/auth"
	"github.com/coderianx/Strata/internal/handlers/organizations"
	"github.com/coderianx/Strata/internal/handlers/teams"
	"github.com/coderianx/Strata/internal/middlewares"
	"github.com/go-chi/chi/v5"
)

// APIRouter builds the top-level router with every route group
// mounted under it.
func APIRouter() *chi.Mux {
	router := chi.NewRouter()

	// auth
	router.Route("/api/v1/auth", func(r chi.Router) {
		r.Post("/register", auth.RegisterHandler)
		r.Post("/login", auth.LoginHandler)
		r.Post("/refresh", auth.RefreshHandler)
		r.With(middlewares.Auth).Get("/me", auth.GetMeHandler)
	})

	// organizations
	router.Route("/api/v1/organizations", func(r chi.Router) {
		r.With(middlewares.Auth).Post("/", organizations.CreateNewOrganizationHandler)
		r.With(middlewares.Auth).Delete("/{organization_id}", organizations.DeleteOrganization)
		r.With(middlewares.Auth).Post("/{organization_id}/invite", organizations.InviteUserToOrganizationHandler)
		r.With(middlewares.Auth).Get("/{organization_id}/members", organizations.OrganizationMembersHandler)
		r.With(middlewares.Auth).Delete("/{organization_id}/members/{member_id}", organizations.RemoveMemberHandler)
		r.With(middlewares.Auth).Get("/{organization_id}/teams", teams.ListOrganizationTeamsHandler)
		r.With(middlewares.Auth).Post("/{organization_id}/teams", teams.CreateNewTeamHandler)
		r.With(middlewares.Auth).Delete("/{organization_id}/teams/{team_id}", teams.DeleteTeamHandler)
		r.With(middlewares.Auth).Post("/{organization_id}/teams/{team_id}/members", teams.AddNewMemberToTeamHandler)
		r.With(middlewares.Auth).Delete("/{organization_id}/teams/{team_id}/members/{member_id}", teams.RemoveMemberFromTeamHandler)
	})

	// my organizations
	router.With(middlewares.Auth).Get("/api/v1/my-organizations", organizations.GetMyOrganizationsHandler)

	// invitations
	router.Route("/api/v1/invitations", func(r chi.Router) {
		r.With(middlewares.Auth).Get("/", organizations.ListMyInvitationsHandler)
		r.With(middlewares.Auth).Post("/{invitation_id}/accept", organizations.AcceptInvitationHandler)
		r.With(middlewares.Auth).Post("/{invitation_id}/reject", organizations.RejectInvitationHandler)
	})

	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	return router
}
