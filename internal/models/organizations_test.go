package models_test

import (
	"encoding/json"
	"testing"

	"github.com/coderianx/Strata/internal/models"
)

func TestCreateNewOrganizationReqJSONFields(t *testing.T) {
	body := `{"name":"Coderian Studio","slug":"coderian-studio","description":"Crafted software.","logo_url":"https://strata.dev/logo.png"}`

	var req models.CreateNewOrganizationReq
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("Unmarshal() = %v, want nil", err)
	}

	want := models.CreateNewOrganizationReq{
		Name:        "Coderian Studio",
		Slug:        "coderian-studio",
		Description: "Crafted software.",
		LogoURL:     "https://strata.dev/logo.png",
	}

	if req != want {
		t.Errorf("Unmarshal() = %+v, want %+v", req, want)
	}
}

func TestInviteSomeoneToOrganizationReqJSONFields(t *testing.T) {
	body := `{"username":"kodcu_42","role":"member"}`

	var req models.InviteSomeoneToOrganizationReq
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("Unmarshal() = %v, want nil", err)
	}

	want := models.InviteSomeoneToOrganizationReq{
		Username: "kodcu_42",
		Role:     "member",
	}

	if req != want {
		t.Errorf("Unmarshal() = %+v, want %+v", req, want)
	}
}
