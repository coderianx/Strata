package models

import (
	"time"
)

// CreateNewOrganizationReq is the body of an organization creation
// request.
type CreateNewOrganizationReq struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	LogoURL     string `json:"logo_url"`

	// Visibility reports whether the organization is public; false
	// means private.
	Visibility bool `json:"visibility"`

	// ShowMembers reports whether non-members may see the member
	// list.
	ShowMembers bool `json:"show_members"`
}

// InviteSomeoneToOrganizationReq is the body of an invite request:
// an existing username and the role to assign on acceptance.
type InviteSomeoneToOrganizationReq struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

// ListReceivedInvitations is one row of the caller's pending
// invitation list.
type ListReceivedInvitations struct {
	ID             int64     `json:"id"`
	OrganizationID int64     `json:"organization_id"`
	UserID         int64     `json:"user_id"`
	InvitedBy      int64     `json:"invited_by"`
	Role           string    `json:"role"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

// MyOrganizations is one row of the caller's organization list.
type MyOrganizations struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	LogoURL     string    `json:"logo_url"`
	CreatedAt   time.Time `json:"created_at"`
}
