// Package config prepares the Strata environment: it creates
// ~/.strata with a generated JWT secret on first run and exposes the
// loaded environment variables.
package config

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

var strataPath string

// Init prepares the Strata environment: it creates the ~/.strata
// folder, writes a freshly generated JWT secret into ~/.strata/.env
// on first run (never overwriting an existing file) and loads the
// file into the process environment.
func Init() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("config: get user home dir: %w", err)
	}

	strataPath = filepath.Join(home, ".strata")

	if err := os.MkdirAll(strataPath, 0755); err != nil {
		return fmt.Errorf("config: create %s folder: %w", strataPath, err)
	}

	envPath := filepath.Join(strataPath, ".env")

	file, err := os.OpenFile(
		envPath,
		os.O_CREATE|os.O_EXCL|os.O_WRONLY,
		0600,
	)

	if err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("config: create env file: %w", err)
	}

	if file != nil {
		secret, err := generateJWTSecret()
		if err != nil {
			file.Close()
			os.Remove(envPath)
			return fmt.Errorf("config: generate jwt secret: %w", err)
		}

		content := fmt.Sprintf("JWT_SECRET=%s\nDB_URL=\n", secret)

		if _, err := file.WriteString(content); err != nil {
			file.Close()
			os.Remove(envPath)
			return fmt.Errorf("config: write env file: %w", err)
		}

		if err := file.Close(); err != nil {
			return fmt.Errorf("config: close env file: %w", err)
		}
	}

	if err := godotenv.Load(envPath); err != nil {
		return fmt.Errorf("config: load env file: %w", err)
	}

	return nil
}

// CheckEnvs reports the required environment variables that are
// empty or missing, naming the env file to fill in.
func CheckEnvs() error {
	var missing []string

	for _, key := range []string{"JWT_SECRET", "DB_URL"} {
		if strings.TrimSpace(os.Getenv(key)) == "" {
			missing = append(missing, key)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf(
			"config: required env vars are empty: %s (fill them in %s)",
			strings.Join(missing, ", "),
			filepath.Join(strataPath, ".env"),
		)
	}

	return nil
}

// GetEnv returns the value of the environment variable name, or an
// empty string when it is not set.
func GetEnv(name string) string {
	return os.Getenv(name)
}

func generateJWTSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(buf), nil
}
