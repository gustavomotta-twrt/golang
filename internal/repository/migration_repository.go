package repository

import (
	"database/sql"
	"fmt"
	"time"
)

type Migration struct {
	ID              int64
	Source          string
	Destination     string
	SourceProjectID string
	DestListID      string
	Status          string
	TotalTasks      int
	CompletedTasks  int
	FailedTasks     int
	StartedAt       time.Time
	CompletedAt     *time.Time
}

type MigrationRepository struct {
	db *sql.DB
}

func NewMigrationRepository(db *sql.DB) *MigrationRepository {
	return &MigrationRepository{db: db}
}

func (r *MigrationRepository) Create(migration *Migration) (int64, error) {
	query := `
	INSERT INTO migrations (source, destination, source_project_id, dest_list_id, status, total_tasks)
        VALUES (?, ?, ?, ?, ?, ?)
	`

	result, err := r.db.Exec(query,
		migration.Source,
		migration.Destination,
		migration.SourceProjectID,
		migration.DestListID,
		migration.Status,
		migration.TotalTasks,
	)

	if err != nil {
		return 0, fmt.Errorf("Error trying to create the migration: %w", err)
	}

	return result.LastInsertId()
}

func (r *MigrationRepository) UpdateProgress(id int64, completed, failed int) error {
	query := `UPDATE migrations SET completed_tasks = ?, failed_tasks = ? WHERE id = ?`
	_, err := r.db.Exec(query, completed, failed, id)
	return err
}

func (r *MigrationRepository) Complete(id int64, status string) error {
	query := `UPDATE migrations SET status = ?, completed_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := r.db.Exec(query, status, id)
	return err
}
