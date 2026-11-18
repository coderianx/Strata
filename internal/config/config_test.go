package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests manipulate process-wide state ($HOME, environment
// variables and the package-level strataPath), so they must not use
// t.Parallel().

// freshHome points $HOME at an empty temp dir so Init creates the
// .strata folder inside the test's own sandbox, never the real one.
func freshHome(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)

	return home
}

func TestInit(t *testing.T) {
	// godotenv.Load skips keys that already exist in the environment,
	// and other tests in this package may have leaked JWT_SECRET or
	// DB_URL, so clear them to observe the values the env file sets.
	os.Unsetenv("JWT_SECRET")
	os.Unsetenv("DB_URL")

	home := freshHome(t)

	if err := Init(); err != nil {
		t.Fatalf("Init() = %v, want nil", err)
	}

	envPath := filepath.Join(home, ".strata", ".env")

	first, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("Init() did not create %s: %v", envPath, err)
	}

	content := string(first)
	if !strings.HasPrefix(content, "JWT_SECRET=") {
		t.Errorf("env file should start with a JWT_SECRET entry, got:\n%s", content)
	}

	if !strings.Contains(content, "\nDB_URL=\n") {
		t.Errorf("env file should contain an empty DB_URL entry, got:\n%s", content)
	}

	info, err := os.Stat(envPath)
	if err != nil {
		t.Fatal(err)
	}

	// The file holds the JWT signing secret, so it must stay private.
	if info.Mode().Perm() != 0o600 {
		t.Errorf("env file mode = %v, want 0600", info.Mode().Perm())
	}

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		t.Fatal("Init() did not export JWT_SECRET from the env file")
	}

	// The secret is base64(RawURL) of 32 random bytes: 43 characters.
	raw, err := base64.RawURLEncoding.DecodeString(secret)
	if err != nil {
		t.Fatalf("JWT_SECRET is not valid base64: %v", err)
	}

	if len(raw) != 32 {
		t.Errorf("JWT_SECRET decodes to %d bytes, want 32", len(raw))
	}

	// A second Init must keep the existing file and secret intact.
	if err := Init(); err != nil {
		t.Fatalf("second Init() = %v, want nil", err)
	}

	second, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("second Init() removed %s: %v", envPath, err)
	}

	if string(second) != content {
		t.Error("second Init() rewrote the env file, want it untouched")
	}

	if got := os.Getenv("JWT_SECRET"); got != secret {
		t.Errorf("JWT_SECRET = %q after second Init(), want %q", got, secret)
	}
}

func TestInitUnreachableHome(t *testing.T) {
	// Point $HOME inside a regular file: MkdirAll cannot create a
	// directory there, so Init must fail without touching anything.
	home := t.TempDir()

	blocker := filepath.Join(home, "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", filepath.Join(blocker, ".strata"))

	err := Init()
	if err == nil {
		t.Fatal("Init() = nil, want error when the home dir is not a directory")
	}

	if !strings.Contains(err.Error(), "folder") {
		t.Errorf("Init() error = %q, want it to mention the folder step", err)
	}
}

func TestInitEnvPathBlocked(t *testing.T) {
	// Make the env file path a directory: O_EXCL tolerates it as an
	// existing path, so the failure surfaces when godotenv tries to
	// read the file.
	home := freshHome(t)

	if err := os.MkdirAll(filepath.Join(home, ".strata", ".env"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := Init()
	if err == nil {
		t.Fatal("Init() = nil, want error when the env file path is a directory")
	}

	if !strings.Contains(err.Error(), "load env file") {
		t.Errorf("Init() error = %q, want it to mention the env file load step", err)
	}
}

func TestCheckEnvs(t *testing.T) {
	tests := []struct {
		name    string
		jwt     string
		dbURL   string
		wantErr string // empty means no error is expected
	}{
		{"complete env", "secret", "postgres://localhost/strata", ""},
		{"missing JWT_SECRET", "", "postgres://localhost/strata", "JWT_SECRET"},
		{"missing DB_URL", "secret", "", "DB_URL"},
		{"missing both", "", "", "JWT_SECRET, DB_URL"},
		{"whitespace DB_URL", "secret", "   ", "DB_URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("JWT_SECRET", tt.jwt)
			t.Setenv("DB_URL", tt.dbURL)

			err := CheckEnvs()
			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("CheckEnvs() = %v, want nil", err)
			case tt.wantErr != "" && err == nil:
				t.Errorf("CheckEnvs() = nil, want error mentioning %s", tt.wantErr)
			case tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr):
				t.Errorf("CheckEnvs() = %q, want it to mention %s", err, tt.wantErr)
			}
		})
	}
}

func TestGetEnv(t *testing.T) {
	t.Setenv("STRATA_TEST_VALUE", "strata-42")

	if got := GetEnv("STRATA_TEST_VALUE"); got != "strata-42" {
		t.Errorf("GetEnv(%q) = %q, want %q", "STRATA_TEST_VALUE", got, "strata-42")
	}

	if got := GetEnv("STRATA_TEST_UNSET"); got != "" {
		t.Errorf("GetEnv(unset key) = %q, want empty", got)
	}
}

func TestGenerateJWTSecret(t *testing.T) {
	a, err := generateJWTSecret()
	if err != nil {
		t.Fatalf("generateJWTSecret() = %v, want nil", err)
	}

	b, err := generateJWTSecret()
	if err != nil {
		t.Fatalf("generateJWTSecret() = %v, want nil", err)
	}

	if a == b {
		t.Error("two generated secrets must differ")
	}

	raw, err := base64.RawURLEncoding.DecodeString(a)
	if err != nil {
		t.Fatalf("generateJWTSecret() = %q, not valid base64: %v", a, err)
	}

	if len(raw) != 32 {
		t.Errorf("secret decodes to %d bytes, want 32", len(raw))
	}
}
