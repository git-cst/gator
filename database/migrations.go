package database

import (
	"database/sql"
	"embed"
	"fmt"

	"gator/config"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

//go:embed sql/migrations
var migrations embed.FS

func RunMigrations(dbConfig *config.DBConfig, db *sql.DB) error {
	dbDriver := dbConfig.DBDriver

	goose.SetBaseFS(migrations)

	if err := goose.SetDialect(dbDriver); err != nil {
		return fmt.Errorf("setting goose dialect: %w", err)
	}

	if err := goose.Up(db, "sql/migrations"); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	return nil
}
