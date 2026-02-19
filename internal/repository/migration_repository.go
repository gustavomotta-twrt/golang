package repository

import (
	"database/sql"
	"fmt"
	"time"
)

type Migration struct {
	Id              int64
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

func (r *MigrationRepository) GetMigrations() ([]Migration, error) {
	query := `
	SELECT * FROM migrations
	`
	rows, err := r.db.Query(query)

	if err != nil {
		return nil, fmt.Errorf("Error trying to get migrations: %w", err)
	}
	defer rows.Close()

	var migrations []Migration

	for rows.Next() {
		var m Migration
		err := rows.Scan(
			&m.Id,
			&m.Source,
			&m.Destination,
			&m.SourceProjectID,
			&m.DestListID,
			&m.Status,
			&m.TotalTasks,
			&m.CompletedTasks,
			&m.FailedTasks,
			&m.StartedAt,
			&m.CompletedAt,
		)
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, m)
	}

	return migrations, nil
}

func (r *MigrationRepository) GetMigration(id string) (Migration, error) {
	query := `
		SELECT * FROM migrations where id = ?
	`

	var m Migration
	err := r.db.QueryRow(query, id).Scan(
		&m.Id,
		&m.Source,
		&m.Destination,
		&m.SourceProjectID,
		&m.DestListID,
		&m.Status,
		&m.TotalTasks,
		&m.CompletedTasks,
		&m.FailedTasks,
		&m.StartedAt,
		&m.CompletedAt,
	)
	if err != nil {
		return Migration{}, fmt.Errorf("Error trying to get migration: %w", err)
	}

	return m, nil
}

func (r *MigrationRepository) UpdateTotalTasks(id int64, totalTasks int) error {
	query := `UPDATE migrations SET total_tasks = ? WHERE id = ?`
	_, err := r.db.Exec(query, totalTasks, id)
	return err
}

func (r *MigrationRepository) UpdateStatus(id int64, status string) error {
	query := `UPDATE migrations SET status = ? WHERE id = ?`
	_, err := r.db.Exec(query, status, id)
	return err
}