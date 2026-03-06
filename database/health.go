package database

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

func WaitForDB(waitTime time.Duration, attempts uint8, db *sql.DB) error {
	for attempt := range attempts {
		if err := db.Ping(); err == nil {
			return nil
		}
		log.Printf("database not ready, attempt %d/%d, retrying in %s...", attempt+1, attempts, waitTime)
		time.Sleep(waitTime)
	}

	return fmt.Errorf("database not ready after %d attempts", attempts)
}
