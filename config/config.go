package config

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"time"
)

type Config struct {
	DBURL        string
	MigrationDir string
	HTTPClient   *http.Client
}

func LoadConfig() (Config, error) {
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		return Config{}, fmt.Errorf("DB_URL environment variable not set")
	}

	migrationDir := os.Getenv("MIGRATION_DIR")
	if migrationDir == "" {
		return Config{}, fmt.Errorf("MIGRATION_DIR environment variable not set")
	}

	_, err := os.Stat(migrationDir)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, fmt.Errorf("migration directory %q does not exist: %w", migrationDir, err)
	} else if err != nil {
		return Config{}, fmt.Errorf("checking migration directory %q: %w", migrationDir, err)
	}

	return Config{
		DBURL:        dbURL,
		MigrationDir: migrationDir,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}
