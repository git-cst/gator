package config

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"time"
)

type Config struct {
	// Database related configuration
	DBURL          string
	DBDriver       string
	MigrationDir   string
	DBConnAttempts uint8
	DBConnWait     time.Duration

	// HTTP related configuration
	HTTPClient *http.Client
}

var validDialects = map[string]bool{
	"postgres": true,
	"mysql":    true,
	"sqlite3":  true,
	"mssql":    true,
}

func LoadConfig() (Config, error) {
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		return Config{}, fmt.Errorf("DB_URL environment variable not set")
	}

	dbDriver := os.Getenv("DB_DRIVER")
	if dbDriver == "" {
		return Config{}, fmt.Errorf("DB_DRIVER environment variable not set")
	}

	if !validDialects[dbDriver] {
		return Config{}, fmt.Errorf("unsupported DB dialect %q", dbDriver)
	}

	migrationDir := os.Getenv("MIGRATION_DIR")
	if migrationDir == "" {
		return Config{}, fmt.Errorf("MIGRATION_DIR environment variable not set")
	}

	dbConnAttemptsStr := os.Getenv("DB_CONNECTION_ATTEMPTS")
	if dbConnAttemptsStr == "" {
		return Config{}, fmt.Errorf("DB_CONNECTION_ATTEMPTS environment variable not set")
	}
	dbConnAttemptsInt, err := strconv.Atoi(dbConnAttemptsStr)
	if err != nil {
		return Config{}, fmt.Errorf("DB_CONNECTION_ATTEMPTS %q is not a valid integer: %w", dbConnAttemptsStr, err)
	}

	dbConnWaitStr := os.Getenv("DB_CONNECTION_WAIT")
	if dbConnWaitStr == "" {
		return Config{}, fmt.Errorf("DB_CONNECTION_WAIT environment variable not set")
	}
	dbConnWaitInt, err := strconv.Atoi(dbConnWaitStr)
	if err != nil {
		return Config{}, fmt.Errorf("DB_CONNECTION_WAIT %q is not a valid integer: %w", dbConnWaitStr, err)
	}

	_, err = os.Stat(migrationDir)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, fmt.Errorf("migration directory %q does not exist: %w", migrationDir, err)
	} else if err != nil {
		return Config{}, fmt.Errorf("checking migration directory %q: %w", migrationDir, err)
	}

	return Config{
		DBURL:          dbURL,
		DBDriver:       dbDriver,
		MigrationDir:   migrationDir,
		DBConnAttempts: uint8(dbConnAttemptsInt),
		DBConnWait:     time.Duration(dbConnWaitInt) * time.Second,

		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}
