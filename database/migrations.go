package database

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

func RunMigrations(migrationDir string, dbDriver string, db *sql.DB) error {
	if err := goose.SetDialect(dbDriver); err != nil {
		return fmt.Errorf("setting goose dialect: %w", err)
	}

	if err := goose.Up(db, migrationDir); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	return nil
}
