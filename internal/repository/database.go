package repository

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func InitDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("Error trying to open DB: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("Error trying to connect: %w", err)
	}

	if err := createTables(db); err != nil {
		return nil, err
	}

	return db, nil
}

func createTables(db *sql.DB) error {
	schema := `
    CREATE TABLE IF NOT EXISTS migrations (
        id               INTEGER PRIMARY KEY AUTOINCREMENT,
        source           TEXT NOT NULL,
        destination      TEXT NOT NULL,
        source_project_id TEXT NOT NULL,
        dest_list_id     TEXT NOT NULL,
        dest_workspace_id TEXT,
        status           TEXT NOT NULL,
        total_tasks      INTEGER DEFAULT 0,
        completed_tasks  INTEGER DEFAULT 0,
        failed_tasks     INTEGER DEFAULT 0,
        started_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
        completed_at     DATETIME
    );

    CREATE TABLE IF NOT EXISTS migration_mappings (
        id           INTEGER PRIMARY KEY AUTOINCREMENT,
        migration_id INTEGER NOT NULL,
        type         TEXT NOT NULL,
        source_value TEXT NOT NULL,
        dest_value   TEXT,
        status       TEXT NOT NULL DEFAULT 'pending',
        metadata     TEXT,
        created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
        updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY (migration_id) REFERENCES migrations(id),
        UNIQUE (migration_id, type, source_value)
    );

    CREATE TABLE IF NOT EXISTS task_mappings (
        id            INTEGER PRIMARY KEY AUTOINCREMENT,
        migration_id  INTEGER NOT NULL,
        source_task_id TEXT NOT NULL,
        dest_task_id  TEXT,
        status        TEXT NOT NULL,
        error_message TEXT,
        created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY (migration_id) REFERENCES migrations(id)
    );
    `

	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("create tables: %w", err)
	}
	return nil
}
