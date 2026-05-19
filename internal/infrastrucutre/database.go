package infrastrucutre

import (
	"database/sql"
	"embed"
	"fmt"
	"log"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite"
)

//go:embed migrate/*.sql
var migrationFiles embed.FS

func NewSqliteStorage() (*sql.DB, error) {
	var db *sql.DB
	var err error

	dsn := "tasks.db"
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxIdleTime(0)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("failed to migrate: %w", err)
	}

	_, err = db.Exec("PRAGMA foreign_keys = ON;")

	return db, nil
}

func runMigrations(db *sql.DB) error {

	sourceDriver, err := iofs.New(migrationFiles, "migrate")
	if err != nil {
		return fmt.Errorf("failed to create source: %w", err)
	}

	var driverName string
	var instance database.Driver
	driverName = "sqlite"
	instance, err = sqlite.WithInstance(db, &sqlite.Config{})

	if err != nil {
		return fmt.Errorf("failed to create db driver: %w", err)
	}

	m, err := migrate.NewWithInstance(
		"iofs",
		sourceDriver,
		driverName,
		instance,
	)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	log.Println("Migrations applied successfully")
	return nil
}
