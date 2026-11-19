package models_test

import (
	"encoding/json"
	"testing"

	"github.com/coderianx/Strata/internal/models"
)

func TestCreateNewTeamReqJSONFields(t *testing.T) {
	body := `{"name":"Backend Team","identifier":"backend-team","description":"Crafted software."}`

	var req models.CreateNewTeamReq
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("Unmarshal() = %v, want nil", err)
	}

	want := models.CreateNewTeamReq{
		Name:        "Backend Team",
		Identifier:  "backend-team",
		Description: "Crafted software.",
	}

	if req != want {
		t.Errorf("Unmarshal() = %+v, want %+v", req, want)
	}
}
