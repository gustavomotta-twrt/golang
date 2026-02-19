package repository

import (
	"database/sql"
	"fmt"
	"time"
)

type TaskMapping struct {
	ID int64
	MigrationID int64
	SourceTaskID string
	DestTaskID string
	Status string
	ErrorMessage string
	CreatedAt time.Time
}

type TaskMappingRepository struct {
	db *sql.DB
}

func NewTaskMappingRepository(db *sql.DB) *TaskMappingRepository {
	return &TaskMappingRepository{db: db}
}

func (r *TaskMappingRepository) Create(mapping *TaskMapping) error {
	query := `
		INSERT INTO task_mappings (migration_id, source_task_id, dest_task_id, status, error_message)
        VALUES (?, ?, ?, ?, ?)
	`
	
	_, err := r.db.Exec(query,
        mapping.MigrationID,
        mapping.SourceTaskID,
        mapping.DestTaskID,
        mapping.Status,
        mapping.ErrorMessage,
    )
	
	if err != nil {
        return fmt.Errorf("erro ao criar mapeamento: %w", err)
    }
    
    return nil
}
