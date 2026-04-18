package database

import (
	"database/sql"
	"fmt"

	"gator/config"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

func RunMigrations(dbConfig *config.DBConfig, db *sql.DB) error {
	dbDriver := dbConfig.DBDriver
	migrationDir := dbConfig.MigrationDir

	if err := goose.SetDialect(dbDriver); err != nil {
		return fmt.Errorf("setting goose dialect: %w", err)
	}

	if err := goose.Up(db, migrationDir); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	return nil
}
