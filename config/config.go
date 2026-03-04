package config

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

type Config struct {
	DBURL      string
	HTTPClient *http.Client
}

func LoadConfig() (Config, error) {
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		return Config{}, fmt.Errorf("DB_URL environment variable not set")
	}

	return Config{
		DBURL: dbURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}
