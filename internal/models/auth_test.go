package models_test

import (
	"encoding/json"
	"testing"

	"github.com/coderianx/Strata/internal/models"
)

// The JSON tags below are the auth API's wire contract: clients send
// these exact keys, so a renamed tag is a breaking change. These tests
// pin the field names.

func TestRegisterReqJSONFields(t *testing.T) {
	body := `{"name":"Kodcu","username":"kodcu_42","email":"kodcu@strata.dev","password":"sifre1234"}`

	var req models.RegisterReq
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("Unmarshal() = %v, want nil", err)
	}

	want := models.RegisterReq{
		Name:     "Kodcu",
		Username: "kodcu_42",
		Email:    "kodcu@strata.dev",
		Password: "sifre1234",
	}

	if req != want {
		t.Errorf("Unmarshal() = %+v, want %+v", req, want)
	}
}

func TestLoginReqJSONFields(t *testing.T) {
	body := `{"email":"kodcu@strata.dev","password":"sifre1234"}`

	var req models.LoginReq
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("Unmarshal() = %v, want nil", err)
	}

	want := models.LoginReq{
		Email:    "kodcu@strata.dev",
		Password: "sifre1234",
	}

	if req != want {
		t.Errorf("Unmarshal() = %+v, want %+v", req, want)
	}
}

func TestRefreshReqJSONFields(t *testing.T) {
	body := `{"refreshToken":"f3a1b2c4"}`

	var req models.RefreshReq
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("Unmarshal() = %v, want nil", err)
	}

	if req.RefreshToken != "f3a1b2c4" {
		t.Errorf("Unmarshal() = %+v, want RefreshToken %q", req, "f3a1b2c4")
	}
}

func TestClaimsJSONFields(t *testing.T) {
	data, err := json.Marshal(models.Claims{UserID: 42, Username: "kodcu"})
	if err != nil {
		t.Fatalf("Marshal() = %v, want nil", err)
	}

	// Struct fields marshal in declaration order and every
	// RegisteredClaims field is omitempty, so the output is exact.
	want := `{"user_id":42,"username":"kodcu"}`

	if string(data) != want {
		t.Errorf("Marshal() = %s, want %s", data, want)
	}
}
