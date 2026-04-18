package database

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"gator/config"
)

func WaitForDB(dbConfig *config.DBConfig, db *sql.DB) error {
	attempts := dbConfig.DBConnAttempts
	waitTime := dbConfig.DBConnWait

	for attempt := range attempts {
		if err := db.Ping(); err == nil {
			return nil
		}
		log.Printf("database not ready, attempt %d/%d, retrying in %s...", attempt+1, attempts, waitTime)
		time.Sleep(waitTime)
	}

	return fmt.Errorf("database not ready after %d attempts", attempts)
}
