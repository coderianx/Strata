package models

// CreateNewTeamReq is the body of a team creation request. The
// identifier is URL-safe and unique inside its organization.
type CreateNewTeamReq struct {
	Name        string `json:"name"`
	Identifier  string `json:"identifier"`
	Description string `json:"description"`
}
