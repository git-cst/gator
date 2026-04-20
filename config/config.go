package config

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"
)

type DBConfig struct {
	DBURL          string
	DBDriver       string
	DBConnAttempts uint8
	DBConnWait     time.Duration
}

type HTTPConfig struct {
	HTTPClient *http.Client
}

type ServiceConfig struct {
	MaxConcurrency uint8
	ServerPort     string
}

type Config struct {
	DBConfig      *DBConfig
	HTTPConfig    *HTTPConfig
	ServiceConfig *ServiceConfig
}

var validDialects = map[string]bool{
	"postgres": true,
	"mysql":    true,
	"sqlite3":  true,
	"mssql":    true,
}

func LoadConfig() (Config, error) {
	// Database config
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

	dbConnAttemptsStr := os.Getenv("DB_CONNECTION_ATTEMPTS")
	if dbConnAttemptsStr == "" {
		return Config{}, fmt.Errorf("DB_CONNECTION_ATTEMPTS environment variable not set")
	}
	dbConnAttemptsInt, err := strconv.Atoi(dbConnAttemptsStr)
	if err != nil {
		return Config{}, fmt.Errorf("DB_CONNECTION_ATTEMPTS %q is not a valid integer: %w", dbConnAttemptsStr, err)
	} else if dbConnAttemptsInt > 255 {
		return Config{}, fmt.Errorf("DB_CONNECTION_ATTEMPTS %q is greater than 255 which is not a valid uint8", dbConnAttemptsInt)
	}

	dbConnWaitStr := os.Getenv("DB_CONNECTION_WAIT")
	if dbConnWaitStr == "" {
		return Config{}, fmt.Errorf("DB_CONNECTION_WAIT environment variable not set")
	}
	dbConnWaitInt, err := strconv.Atoi(dbConnWaitStr)
	if err != nil {
		return Config{}, fmt.Errorf("DB_CONNECTION_WAIT %q is not a valid integer: %w", dbConnWaitStr, err)
	}

	// Service Config
	maxConcStr := os.Getenv("MAX_CONCURRENCY")
	if maxConcStr == "" {
		return Config{}, fmt.Errorf("MAX_CONCURRENCY environment variable not set")
	}
	maxConcInt, err := strconv.Atoi(maxConcStr)
	if err != nil {
		return Config{}, fmt.Errorf("MAX_CONCURRENCY %q is not a valid integer: %w", maxConcStr, err)
	} else if maxConcInt > 255 {
		return Config{}, fmt.Errorf("MAX_CONCURRENCY %q is greater than 255 which is not a valid uint8", maxConcInt)
	}

	serverPort := os.Getenv("SERVER_PORT")
	if serverPort == "" {
		return Config{}, fmt.Errorf("SERVER_PORT environment variable not set")
	}

	return Config{
		DBConfig: &DBConfig{
			DBURL:          dbURL,
			DBDriver:       dbDriver,
			DBConnAttempts: uint8(dbConnAttemptsInt),
			DBConnWait:     time.Duration(dbConnWaitInt) * time.Second,
		},

		HTTPConfig: &HTTPConfig{
			HTTPClient: &http.Client{
				Timeout: 30 * time.Second,
			},
		},

		ServiceConfig: &ServiceConfig{
			MaxConcurrency: uint8(maxConcInt),
			ServerPort:     serverPort,
		},
	}, nil
}
