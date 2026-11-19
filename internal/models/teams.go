package models

import "time"

// CreateNewTeamReq is the body of a team creation request. The
// identifier is URL-safe and unique inside its organization.
type CreateNewTeamReq struct {
	Name        string `json:"name"`
	Identifier  string `json:"identifier"`
	Description string `json:"description"`
}

// AddNewMemberToTeamReq is the body of an add-team-member request:
// an existing username and the role to assign.
type AddNewMemberToTeamReq struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

// OrganizationTeam is one row of an organization's team list.
type OrganizationTeam struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Identifier  string    `json:"identifier"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}
