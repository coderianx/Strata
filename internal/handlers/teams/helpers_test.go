package teams

import (
	"strings"
	"testing"

	"github.com/coderianx/Strata/internal/models"
)

func TestValidateTeamIdentifier(t *testing.T) {
	cases := []struct {
		name       string
		identifier string
		wantErr    bool
	}{
		{"valid simple", "backend", false},
		{"valid with digits/hyphen", "team-42", false},
		{"valid single hyphens", "a-b-c", false},
		{"reserved name is fine inside an organization", "admin", false},
		{"uppercase", "Backend", true},
		{"starts with digit", "1backend", true},
		{"starts with hyphen", "-backend", true},
		{"ends with hyphen", "backend-", true},
		{"consecutive hyphens", "back--end", true},
		{"underscore", "back_end", true},
		{"space", "back end", true},
		{"too short", "ab", true},
		{"too long", "abcdefghijklmnopqrstuvwxyz012345", true},
		{"empty", "", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateTeamIdentifier(c.identifier)
			if (err != nil) != c.wantErr {
				t.Errorf("validateTeamIdentifier(%q) = %v, wantErr %v", c.identifier, err, c.wantErr)
			}
		})
	}
}

func TestNormalizeCreateNewTeamReq(t *testing.T) {
	req := models.CreateNewTeamReq{
		Name:        "  Backend Team  ",
		Identifier:  "  BACKEND-TEAM  ",
		Description: "  Crafted software.  ",
	}

	normalizeCreateNewTeamReq(&req)

	if req.Name != "Backend Team" {
		t.Errorf("name = %q, want %q", req.Name, "Backend Team")
	}

	if req.Identifier != "backend-team" {
		t.Errorf("identifier = %q, want %q", req.Identifier, "backend-team")
	}

	if req.Description != "Crafted software." {
		t.Errorf("description = %q, want %q", req.Description, "Crafted software.")
	}
}

func TestNormalizeThenValidateCreateNewTeamReq(t *testing.T) {
	req := models.CreateNewTeamReq{
		Name:        " Backend Team ",
		Identifier:  "  BACKEND-TEAM  ",
		Description: " We craft software. ",
	}

	normalizeCreateNewTeamReq(&req)

	if err := validateCreateNewTeamReq(req); err != nil {
		t.Errorf("expected request to be valid after normalize, got: %v", err)
	}
}

func TestValidateCreateNewTeamReq(t *testing.T) {
	cases := []struct {
		name    string
		req     models.CreateNewTeamReq
		wantErr bool
	}{
		{
			"valid with required fields only",
			models.CreateNewTeamReq{Name: "Backend Team", Identifier: "backend-team"},
			false,
		},
		{
			"valid with all fields",
			models.CreateNewTeamReq{Name: "Backend Team", Identifier: "backend-team", Description: "Crafted software."},
			false,
		},
		{"missing name", models.CreateNewTeamReq{Identifier: "backend-team"}, true},
		{"name too long", models.CreateNewTeamReq{Name: strings.Repeat("a", 81), Identifier: "backend-team"}, true},
		{"missing identifier", models.CreateNewTeamReq{Name: "Backend Team"}, true},
		{"uppercase identifier without normalize", models.CreateNewTeamReq{Name: "Backend Team", Identifier: "Backend"}, true},
		{
			"description too long",
			models.CreateNewTeamReq{Name: "Backend Team", Identifier: "backend-team", Description: strings.Repeat("a", 501)},
			true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateCreateNewTeamReq(c.req)
			if (err != nil) != c.wantErr {
				t.Errorf("validateCreateNewTeamReq(%+v) = %v, wantErr %v", c.req, err, c.wantErr)
			}
		})
	}
}
