package repository

import (
	"database/sql"
	"fmt"
	"strings"

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

	if err := runMigrations(db); err != nil {
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
        dest_space_id    TEXT,
        status           TEXT NOT NULL,
        total_tasks      INTEGER DEFAULT 0,
        completed_tasks  INTEGER DEFAULT 0,
        failed_tasks     INTEGER DEFAULT 0,
        started_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
        completed_at     DATETIME
    );

    CREATE TABLE IF NOT EXISTS migration_mappings (
        id                  INTEGER PRIMARY KEY AUTOINCREMENT,
        migration_id        INTEGER NOT NULL,
        type                TEXT NOT NULL,
        source_value        TEXT NOT NULL,
        dest_value          TEXT,
        source_container_id TEXT,
        status              TEXT NOT NULL DEFAULT 'pending',
        metadata            TEXT,
        created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
        updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY (migration_id) REFERENCES migrations(id),
        UNIQUE (migration_id, type, source_value, source_container_id)
    );

    CREATE UNIQUE INDEX IF NOT EXISTS idx_migration_mappings_global_unique
        ON migration_mappings (migration_id, type, source_value)
        WHERE source_container_id IS NULL;

    CREATE INDEX IF NOT EXISTS idx_mm_migration_container
        ON migration_mappings (migration_id, source_container_id);

    CREATE INDEX IF NOT EXISTS idx_mm_migration_status
        ON migration_mappings (migration_id, status);

    CREATE TABLE IF NOT EXISTS container_mappings (
        id           INTEGER PRIMARY KEY AUTOINCREMENT,
        migration_id INTEGER NOT NULL,
        source_id    TEXT NOT NULL,
        source_name  TEXT NOT NULL,
        dest_id      TEXT,
        dest_name    TEXT,
        status       TEXT NOT NULL DEFAULT 'pending',
        enabled      INTEGER NOT NULL DEFAULT 1,
        FOREIGN KEY (migration_id) REFERENCES migrations(id),
        UNIQUE (migration_id, source_id)
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

func runMigrations(db *sql.DB) error {
	if err := migrateAddSourceContainerID(db); err != nil {
		return fmt.Errorf("migration add source_container_id: %w", err)
	}

	// Ensure enabled column exists on container_mappings (legacy migration)
	if _, err := db.Exec(`ALTER TABLE container_mappings ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("alter container_mappings add enabled: %w", err)
		}
	}

	return nil
}

// migrateAddSourceContainerID recreates migration_mappings to add source_container_id
// if the column does not yet exist.
func migrateAddSourceContainerID(db *sql.DB) error {
	rows, err := db.Query("PRAGMA table_info(migration_mappings)")
	if err != nil {
		return fmt.Errorf("pragma table_info: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("scan pragma row: %w", err)
		}
		if name == "source_container_id" {
			return nil // already migrated
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Recreate table with new schema inside a transaction.
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	stmts := []string{
		`CREATE TABLE migration_mappings_new (
            id                  INTEGER PRIMARY KEY AUTOINCREMENT,
            migration_id        INTEGER NOT NULL,
            type                TEXT NOT NULL,
            source_value        TEXT NOT NULL,
            dest_value          TEXT,
            source_container_id TEXT,
            status              TEXT NOT NULL DEFAULT 'pending',
            metadata            TEXT,
            created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
            updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY (migration_id) REFERENCES migrations(id),
            UNIQUE (migration_id, type, source_value, source_container_id)
        )`,
		`INSERT INTO migration_mappings_new
            (id, migration_id, type, source_value, dest_value, source_container_id, status, metadata, created_at, updated_at)
         SELECT id, migration_id, type, source_value, dest_value, NULL, status, metadata, created_at, updated_at
         FROM migration_mappings`,
		`DROP TABLE migration_mappings`,
		`ALTER TABLE migration_mappings_new RENAME TO migration_mappings`,
	}

	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			_ = tx.Rollback() //nolint:errcheck // rollback error is secondary to the transaction error above
			return fmt.Errorf("migration stmt: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}

	// Create partial unique index for global (NULL container) mappings.
	_, err = db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_migration_mappings_global_unique
        ON migration_mappings (migration_id, type, source_value)
        WHERE source_container_id IS NULL`)
	return err
}
