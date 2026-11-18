package organizations

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/coderianx/Strata/internal/models"
)

const (
	// maxOrgNameLength bounds the display name; PostgreSQL TEXT would
	// accept far more, but listings and headers should not.
	maxOrgNameLength = 80

	minSlugLength = 3
	maxSlugLength = 30

	maxDescriptionLength = 500
)

var orgSlugRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// reservedOrgSlugs cannot be used by organizations. They mirror
// reservedUsernames in the auth package: slugs end up in URLs, so
// confusing names are blocked. Extend as needed.
var reservedOrgSlugs = map[string]struct{}{
	"admin":         {},
	"administrator": {},
	"anonymous":     {},
	"api":           {},
	"auth":          {},
	"billing":       {},
	"help":          {},
	"hostmaster":    {},
	"info":          {},
	"login":         {},
	"logout":        {},
	"mail":          {},
	"manager":       {},
	"mod":           {},
	"moderator":     {},
	"null":          {},
	"password":      {},
	"postmaster":    {},
	"profile":       {},
	"register":      {},
	"root":          {},
	"security":      {},
	"settings":      {},
	"staff":         {},
	"strata":        {},
	"support":       {},
	"system":        {},
	"undefined":     {},
	"webmaster":     {},
	"www":           {},
}

// validateOrgSlug enforces URL-safe slug rules, mirroring the
// username rules used by the auth package:
//   - 3-30 characters,
//   - only lowercase letters, digits and hyphens,
//   - must start with a letter,
//   - cannot end with a hyphen,
//   - no consecutive hyphens,
//   - cannot be reserved (admin, root, api, ...).
func validateOrgSlug(slug string) error {
	if len(slug) < minSlugLength {
		return fmt.Errorf("slug must be at least %d characters", minSlugLength)
	}

	if len(slug) > maxSlugLength {
		return fmt.Errorf("slug must be at most %d characters", maxSlugLength)
	}

	if !orgSlugRe.MatchString(slug) {
		return errors.New("slug can only contain lowercase letters, digits and hyphens")
	}

	if slug[0] < 'a' || slug[0] > 'z' {
		return errors.New("slug must start with a letter")
	}

	if slug[len(slug)-1] == '-' {
		return errors.New("slug cannot end with a hyphen")
	}

	if strings.Contains(slug, "--") {
		return errors.New("slug cannot contain consecutive hyphens")
	}

	if _, ok := reservedOrgSlugs[slug]; ok {
		return errors.New("this slug is reserved")
	}

	return nil
}

// normalizeCreateNewOrganizationReq trims the text fields and
// lowercases the slug so the exact values that pass validation are
// the ones stored.
func normalizeCreateNewOrganizationReq(req *models.CreateNewOrganizationReq) {
	req.Name = strings.TrimSpace(req.Name)
	req.Slug = strings.ToLower(strings.TrimSpace(req.Slug))
	req.Description = strings.TrimSpace(req.Description)
	req.LogoURL = strings.TrimSpace(req.LogoURL)
}

// validateCreateNewOrganizationReq checks that the request can be
// stored as-is: name present and bounded, slug URL-safe, description
// bounded, and logo_url an absolute http(s) URL when provided.
func validateCreateNewOrganizationReq(req models.CreateNewOrganizationReq) error {
	if req.Name == "" {
		return errors.New("name is required")
	}

	if len(req.Name) > maxOrgNameLength {
		return fmt.Errorf("name must be at most %d characters", maxOrgNameLength)
	}

	if err := validateOrgSlug(req.Slug); err != nil {
		return err
	}

	if len(req.Description) > maxDescriptionLength {
		return fmt.Errorf("description must be at most %d characters", maxDescriptionLength)
	}

	if req.LogoURL != "" &&
		!strings.HasPrefix(req.LogoURL, "http://") &&
		!strings.HasPrefix(req.LogoURL, "https://") {
		return errors.New("logo_url must be an http(s) URL")
	}

	return nil
}

// allowedRoles lists the roles an invitation can assign. They mirror
// the "owner or member" comment on the organization_members.role
// column.
var allowedRoles = map[string]struct{}{
	"member": {},
	"owner":  {},
}

// normalizeInviteSomeoneToOrganizationReq trims the fields and
// lowercases the role so the exact values that pass validation are
// the ones stored. An omitted role defaults to "member", mirroring
// the role column's DEFAULT.
func normalizeInviteSomeoneToOrganizationReq(req *models.InviteSomeoneToOrganizationReq) {
	req.Username = strings.TrimSpace(req.Username)
	req.Role = strings.ToLower(strings.TrimSpace(req.Role))

	if req.Role == "" {
		req.Role = "member"
	}
}

// validateInviteSomeoneToOrganizationReq only checks that the
// username is present, not the registration rules: like LoginHandler
// with emails, an invalid username can never match a stored user, so
// the lookup answers it. The role must be one of the assignable
// roles.
func validateInviteSomeoneToOrganizationReq(req models.InviteSomeoneToOrganizationReq) error {
	if req.Username == "" {
		return errors.New("username is required")
	}

	if _, ok := allowedRoles[req.Role]; !ok {
		return errors.New("role must be one of: member, owner")
	}

	return nil
}
