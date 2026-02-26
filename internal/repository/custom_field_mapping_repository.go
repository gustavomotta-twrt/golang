package repository

import (
	"database/sql"
	"fmt"
	"time"
)

type CustomFieldMappingRecord struct {
	ID               int64
	MigrationID      int64
	SourceFieldID    string
	SourceFieldName  string
	SourceFieldType  string
	DestFieldID      string
	DestFieldName    string
	DestFieldType    string
	Degraded         bool
	CreatedAt        time.Time
}

type CustomFieldMappingRepository struct {
	db *sql.DB
}

func NewCustomFieldMappingRepository(db *sql.DB) *CustomFieldMappingRepository {
	return &CustomFieldMappingRepository{db: db}
}

func (r *CustomFieldMappingRepository) Create(mapping *CustomFieldMappingRecord) (int64, error) {
	query := `
		INSERT INTO custom_field_mappings (
			migration_id, source_field_id, source_field_name, source_field_type,
			dest_field_id, dest_field_name, dest_field_type, degraded
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	degraded := 0
	if mapping.Degraded {
		degraded = 1
	}
	result, err := r.db.Exec(query,
		mapping.MigrationID,
		mapping.SourceFieldID,
		mapping.SourceFieldName,
		mapping.SourceFieldType,
		mapping.DestFieldID,
		mapping.DestFieldName,
		mapping.DestFieldType,
		degraded,
	)
	if err != nil {
		return 0, fmt.Errorf("create custom field mapping: %w", err)
	}
	return result.LastInsertId()
}

func (r *CustomFieldMappingRepository) FindByMigrationID(migrationID int64) ([]CustomFieldMappingRecord, error) {
	query := `
		SELECT id, migration_id, source_field_id, source_field_name, source_field_type,
			dest_field_id, dest_field_name, dest_field_type, degraded, created_at
		FROM custom_field_mappings
		WHERE migration_id = ?
		ORDER BY id
	`
	rows, err := r.db.Query(query, migrationID)
	if err != nil {
		return nil, fmt.Errorf("find custom field mappings by migration id: %w", err)
	}
	defer rows.Close()

	var records []CustomFieldMappingRecord
	for rows.Next() {
		var rec CustomFieldMappingRecord
		var degraded int
		err := rows.Scan(
			&rec.ID,
			&rec.MigrationID,
			&rec.SourceFieldID,
			&rec.SourceFieldName,
			&rec.SourceFieldType,
			&rec.DestFieldID,
			&rec.DestFieldName,
			&rec.DestFieldType,
			&degraded,
			&rec.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan custom field mapping: %w", err)
		}
		rec.Degraded = degraded != 0
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate custom field mappings: %w", err)
	}
	return records, nil
}

func (r *CustomFieldMappingRepository) DeleteByMigrationID(migrationID int64) error {
	_, err := r.db.Exec(`DELETE FROM custom_field_option_mappings WHERE field_mapping_id IN (SELECT id FROM custom_field_mappings WHERE migration_id = ?)`, migrationID)
	if err != nil {
		return fmt.Errorf("delete custom field option mappings by migration id: %w", err)
	}
	_, err = r.db.Exec(`DELETE FROM custom_field_mappings WHERE migration_id = ?`, migrationID)
	if err != nil {
		return fmt.Errorf("delete custom field mappings by migration id: %w", err)
	}
	return nil
}
