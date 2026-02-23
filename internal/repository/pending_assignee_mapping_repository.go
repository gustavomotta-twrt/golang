package repository

import (
	"database/sql"
	"fmt"
	"time"
)

type PendingAssigneeMapping struct {
	ID int64
	MigrationID int64
	SourceUserId string
	SourceUserName string
	SourceUserEmail string
	CreatedAt time.Time
}

type PendingAssigneeMappingRepository struct {
	db *sql.DB
}

func NewPendingAssigneeMappingRepository(db *sql.DB) *PendingAssigneeMappingRepository {
	return &PendingAssigneeMappingRepository{db: db}
}

func (r *PendingAssigneeMappingRepository) Create(mapping *PendingAssigneeMapping) error {
	query := `
		INSERT INTO pending_assignee_mappings (migration_id, source_user_id, source_user_name, source_user_email) VALUES (?, ?, ?, ?)
	`

	_, err := r.db.Exec(query,
		mapping.MigrationID,
		mapping.SourceUserId,
		mapping.SourceUserName,
		mapping.SourceUserEmail,
	)

	if err != nil {
		return fmt.Errorf("Error trying to create the map: %w", err)
	}

	return nil
}

func (r *PendingAssigneeMappingRepository) GetByMigrationID(migrationID int64) ([]PendingAssigneeMapping, error) {
	query := `SELECT id, migration_id, source_user_id, source_user_name, source_user_email, created_at 
		FROM pending_assignee_mappings WHERE migration_id = ?`

	rows, err := r.db.Query(query, migrationID)
	if err != nil {
		return nil, fmt.Errorf("get pending assignee mappings: %w", err)
	}
	defer rows.Close()

	var mappings []PendingAssigneeMapping
	for rows.Next() {
		var m PendingAssigneeMapping
		err := rows.Scan(&m.ID, &m.MigrationID, &m.SourceUserId, &m.SourceUserName, &m.SourceUserEmail, &m.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan pending assignee mapping: %w", err)
		}
		mappings = append(mappings, m)
	}
	return mappings, nil
}

func (r *PendingAssigneeMappingRepository) DeleteByMigrationID(migrationID int64) error {
	query := `DELETE FROM pending_assignee_mappings WHERE migration_id = ?`
	_, err := r.db.Exec(query, migrationID)
	if err != nil {
		return fmt.Errorf("delete pending assignee mappings: %w", err)
	}
	return nil
}