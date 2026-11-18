package organizations

import (
	"strings"
	"testing"

	"github.com/coderianx/Strata/internal/models"
)

func TestValidateOrgSlug(t *testing.T) {
	cases := []struct {
		name    string
		slug    string
		wantErr bool
	}{
		{"valid simple", "coderian", false},
		{"valid with digits/hyphen", "coderian-42", false},
		{"valid single hyphens", "a-b-c", false},
		{"uppercase", "Coderian", true},
		{"starts with digit", "1coderian", true},
		{"starts with hyphen", "-coderian", true},
		{"ends with hyphen", "coderian-", true},
		{"consecutive hyphens", "coder--ian", true},
		{"underscore", "coder_ian", true},
		{"space", "coder ian", true},
		{"too short", "ab", true},
		{"too long", "abcdefghijklmnopqrstuvwxyz012345", true},
		{"empty", "", true},
		{"reserved admin", "admin", true},
		{"reserved moderator", "moderator", true},
		{"reserved only exact match", "coderian-admin", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateOrgSlug(c.slug)
			if (err != nil) != c.wantErr {
				t.Errorf("validateOrgSlug(%q) = %v, wantErr %v", c.slug, err, c.wantErr)
			}
		})
	}
}

func TestNormalizeCreateNewOrganizationReq(t *testing.T) {
	req := models.CreateNewOrganizationReq{
		Name:        "  Coderian Studio  ",
		Slug:        "  CODERIAN-STUDIO  ",
		Description: "  Crafted software.  ",
		LogoURL:     "  https://strata.dev/logo.png  ",
	}

	normalizeCreateNewOrganizationReq(&req)

	if req.Name != "Coderian Studio" {
		t.Errorf("name = %q, want %q", req.Name, "Coderian Studio")
	}

	if req.Slug != "coderian-studio" {
		t.Errorf("slug = %q, want %q", req.Slug, "coderian-studio")
	}

	if req.Description != "Crafted software." {
		t.Errorf("description = %q, want %q", req.Description, "Crafted software.")
	}

	if req.LogoURL != "https://strata.dev/logo.png" {
		t.Errorf("logo_url = %q, want %q", req.LogoURL, "https://strata.dev/logo.png")
	}
}

func TestNormalizeThenValidateCreateNewOrganizationReq(t *testing.T) {
	req := models.CreateNewOrganizationReq{
		Name:        " Coderian Studio ",
		Slug:        "  CODERIAN-STUDIO  ",
		Description: " We craft software. ",
		LogoURL:     " https://strata.dev/logo.png ",
	}

	normalizeCreateNewOrganizationReq(&req)

	if err := validateCreateNewOrganizationReq(req); err != nil {
		t.Errorf("expected request to be valid after normalize, got: %v", err)
	}
}

func TestValidateCreateNewOrganizationReq(t *testing.T) {
	cases := []struct {
		name    string
		req     models.CreateNewOrganizationReq
		wantErr bool
	}{
		{
			"valid with required fields only",
			models.CreateNewOrganizationReq{Name: "Coderian Studio", Slug: "coderian-studio"},
			false,
		},
		{
			"valid with all fields",
			models.CreateNewOrganizationReq{
				Name:        "Coderian Studio",
				Slug:        "coderian-studio",
				Description: "We craft software.",
				LogoURL:     "https://strata.dev/logo.png",
			},
			false,
		},
		{
			"missing name",
			models.CreateNewOrganizationReq{Slug: "coderian-studio"},
			true,
		},
		{
			"name too long",
			models.CreateNewOrganizationReq{Name: strings.Repeat("a", 81), Slug: "coderian-studio"},
			true,
		},
		{
			"missing slug",
			models.CreateNewOrganizationReq{Name: "Coderian Studio"},
			true,
		},
		{
			"uppercase slug without normalize",
			models.CreateNewOrganizationReq{Name: "Coderian Studio", Slug: "Coderian"},
			true,
		},
		{
			"description too long",
			models.CreateNewOrganizationReq{
				Name:        "Coderian Studio",
				Slug:        "coderian-studio",
				Description: strings.Repeat("a", 501),
			},
			true,
		},
		{
			"logo_url without scheme",
			models.CreateNewOrganizationReq{Name: "Coderian Studio", Slug: "coderian-studio", LogoURL: "strata.dev/logo.png"},
			true,
		},
		{
			"logo_url with ftp scheme",
			models.CreateNewOrganizationReq{Name: "Coderian Studio", Slug: "coderian-studio", LogoURL: "ftp://strata.dev/logo.png"},
			true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateCreateNewOrganizationReq(c.req)
			if (err != nil) != c.wantErr {
				t.Errorf("validateCreateNewOrganizationReq(%+v) = %v, wantErr %v", c.req, err, c.wantErr)
			}
		})
	}
}
