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
	DestWorkspaceID string
	StatusMappings  string
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
	INSERT INTO migrations (source, destination, source_project_id, dest_list_id, dest_workspace_id, status_mappings, status, total_tasks)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := r.db.Exec(query,
		migration.Source,
		migration.Destination,
		migration.SourceProjectID,
		migration.DestListID,
		migration.DestWorkspaceID,
		migration.StatusMappings,
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
	query := `SELECT id, source, destination, source_project_id, dest_list_id, dest_workspace_id, status_mappings, status, total_tasks, completed_tasks, failed_tasks, started_at, completed_at FROM migrations`
	rows, err := r.db.Query(query)

	if err != nil {
		return nil, fmt.Errorf("Error trying to get migrations: %w", err)
	}
	defer rows.Close()

	var migrations []Migration

	for rows.Next() {
		var m Migration
		var destWorkspaceID, statusMappings sql.NullString
		err := rows.Scan(
			&m.Id,
			&m.Source,
			&m.Destination,
			&m.SourceProjectID,
			&m.DestListID,
			&destWorkspaceID,
			&statusMappings,
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
		if destWorkspaceID.Valid {
			m.DestWorkspaceID = destWorkspaceID.String
		}
		if statusMappings.Valid {
			m.StatusMappings = statusMappings.String
		}
		migrations = append(migrations, m)
	}

	return migrations, nil
}

func (r *MigrationRepository) GetMigration(id int64) (Migration, error) {
	query := `SELECT id, source, destination, source_project_id, dest_list_id, dest_workspace_id, status_mappings, status, total_tasks, completed_tasks, failed_tasks, started_at, completed_at FROM migrations WHERE id = ?`

	var m Migration
	var destWorkspaceID, statusMappings sql.NullString
	err := r.db.QueryRow(query, id).Scan(
		&m.Id,
		&m.Source,
		&m.Destination,
		&m.SourceProjectID,
		&m.DestListID,
		&destWorkspaceID,
		&statusMappings,
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
	if destWorkspaceID.Valid {
		m.DestWorkspaceID = destWorkspaceID.String
	}
	if statusMappings.Valid {
		m.StatusMappings = statusMappings.String
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