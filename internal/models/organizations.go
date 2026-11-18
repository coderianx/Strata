package models

import (
	"time"
)

type CreateNewOrganizationReq struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	LogoURL     string `json:"logo_url"`
	Visibility  bool   `json:"visibility"`
	ShowMembers bool   `json:"show_members"`
}

type InviteSomeoneToOrganizationReq struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

type ListReceivedInvitations struct {
	ID             int64     `json:"id"`
	OrganizationID int64     `json:"organization_id"`
	UserID         int64     `json:"user_id"`
	InvitedBy      int64     `json:"invited_by"`
	Role           string    `json:"role"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

type MyOrganizations struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	LogoURL     string    `json:"logo_url"`
	CreatedAt   time.Time `json:"created_at"`
}
