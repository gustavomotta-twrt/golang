package repository

import (
	"database/sql"
	"fmt"
)

type CustomFieldOptionMappingRecord struct {
	ID                int64
	FieldMappingID    int64
	SourceOptionID    string
	SourceOptionName  string
	DestOptionID      string
	DestOptionName    string
}

type CustomFieldOptionMappingRepository struct {
	db *sql.DB
}

func NewCustomFieldOptionMappingRepository(db *sql.DB) *CustomFieldOptionMappingRepository {
	return &CustomFieldOptionMappingRepository{db: db}
}

func (r *CustomFieldOptionMappingRepository) Create(mapping *CustomFieldOptionMappingRecord) error {
	query := `
		INSERT INTO custom_field_option_mappings (
			field_mapping_id, source_option_id, source_option_name, dest_option_id, dest_option_name
		) VALUES (?, ?, ?, ?, ?)
	`
	_, err := r.db.Exec(query,
		mapping.FieldMappingID,
		mapping.SourceOptionID,
		mapping.SourceOptionName,
		mapping.DestOptionID,
		mapping.DestOptionName,
	)
	if err != nil {
		return fmt.Errorf("create custom field option mapping: %w", err)
	}
	return nil
}

func (r *CustomFieldOptionMappingRepository) BulkCreate(mappings []CustomFieldOptionMappingRecord) error {
	if len(mappings) == 0 {
		return nil
	}
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction for bulk create option mappings: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.Prepare(`
		INSERT INTO custom_field_option_mappings (
			field_mapping_id, source_option_id, source_option_name, dest_option_id, dest_option_name
		) VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare bulk insert option mappings: %w", err)
	}
	defer stmt.Close()

	for i := range mappings {
		m := &mappings[i]
		_, err = stmt.Exec(m.FieldMappingID, m.SourceOptionID, m.SourceOptionName, m.DestOptionID, m.DestOptionName)
		if err != nil {
			err = fmt.Errorf("bulk create option mapping at index %d: %w", i, err)
			return err
		}
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("commit bulk create option mappings: %w", err)
	}
	return nil
}

func (r *CustomFieldOptionMappingRepository) FindByFieldMappingID(fieldMappingID int64) ([]CustomFieldOptionMappingRecord, error) {
	query := `
		SELECT id, field_mapping_id, source_option_id, source_option_name, dest_option_id, dest_option_name
		FROM custom_field_option_mappings
		WHERE field_mapping_id = ?
		ORDER BY id
	`
	rows, err := r.db.Query(query, fieldMappingID)
	if err != nil {
		return nil, fmt.Errorf("find custom field option mappings by field mapping id: %w", err)
	}
	defer rows.Close()

	var records []CustomFieldOptionMappingRecord
	for rows.Next() {
		var rec CustomFieldOptionMappingRecord
		err := rows.Scan(
			&rec.ID,
			&rec.FieldMappingID,
			&rec.SourceOptionID,
			&rec.SourceOptionName,
			&rec.DestOptionID,
			&rec.DestOptionName,
		)
		if err != nil {
			return nil, fmt.Errorf("scan custom field option mapping: %w", err)
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate custom field option mappings: %w", err)
	}
	return records, nil
}
