package teams

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/coderianx/Strata/internal/models"
)

const (
	// maxTeamNameLength bounds the display name, like organization
	// names.
	maxTeamNameLength = 80

	minTeamIdentifierLength = 3
	maxTeamIdentifierLength = 30

	maxTeamDescriptionLength = 500
)

var teamIdentifierRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// validateTeamIdentifier enforces URL-safe identifier rules,
// mirroring the organization slug rules:
//   - 3-30 characters,
//   - only lowercase letters, digits and hyphens,
//   - must start with a letter,
//   - cannot end with a hyphen,
//   - no consecutive hyphens.
//
// Unlike global slugs, identifiers live inside their organization's
// namespace (UNIQUE (organization_id, identifier)), so no reserved
// list is needed: an "admin" team inside one organization confuses
// no global URL.
func validateTeamIdentifier(identifier string) error {
	if len(identifier) < minTeamIdentifierLength {
		return fmt.Errorf("identifier must be at least %d characters", minTeamIdentifierLength)
	}

	if len(identifier) > maxTeamIdentifierLength {
		return fmt.Errorf("identifier must be at most %d characters", maxTeamIdentifierLength)
	}

	if !teamIdentifierRe.MatchString(identifier) {
		return errors.New("identifier can only contain lowercase letters, digits and hyphens")
	}

	if identifier[0] < 'a' || identifier[0] > 'z' {
		return errors.New("identifier must start with a letter")
	}

	if identifier[len(identifier)-1] == '-' {
		return errors.New("identifier cannot end with a hyphen")
	}

	if strings.Contains(identifier, "--") {
		return errors.New("identifier cannot contain consecutive hyphens")
	}

	return nil
}

// normalizeCreateNewTeamReq trims the text fields and lowercases the
// identifier so the exact values that pass validation are the ones
// stored.
func normalizeCreateNewTeamReq(req *models.CreateNewTeamReq) {
	req.Name = strings.TrimSpace(req.Name)
	req.Identifier = strings.ToLower(strings.TrimSpace(req.Identifier))
	req.Description = strings.TrimSpace(req.Description)
}

// validateCreateNewTeamReq checks that the request can be stored
// as-is: name present and bounded, identifier URL-safe, description
// bounded.
func validateCreateNewTeamReq(req models.CreateNewTeamReq) error {
	if req.Name == "" {
		return errors.New("name is required")
	}

	if len(req.Name) > maxTeamNameLength {
		return fmt.Errorf("name must be at most %d characters", maxTeamNameLength)
	}

	if err := validateTeamIdentifier(req.Identifier); err != nil {
		return err
	}

	if len(req.Description) > maxTeamDescriptionLength {
		return fmt.Errorf("description must be at most %d characters", maxTeamDescriptionLength)
	}

	return nil
}
