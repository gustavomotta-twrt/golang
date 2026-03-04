package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type MappingType string

const (
	MappingTypeStatus      MappingType = "status"
	MappingTypePriority    MappingType = "priority"
	MappingTypeAssignee    MappingType = "assignee"
	MappingTypeCustomField MappingType = "custom_field"
)

type MappingStatus string

const (
	MappingStatusPending  MappingStatus = "pending"
	MappingStatusMapped   MappingStatus = "mapped"
	MappingStatusEnabled  MappingStatus = "enabled"
	MappingStatusDisabled MappingStatus = "disabled"
)

type AssigneeMetadata struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type CustomFieldMetadata struct {
	Name      string `json:"name"`
	FieldType string `json:"field_type"`
}

type CustomFieldRow struct {
	FieldID   string
	FieldName string
	FieldType string
	Enabled   bool
}

type MigrationMapping struct {
	ID          int64
	MigrationID int64
	Type        MappingType
	SourceValue string
	DestValue   *string
	Status      MappingStatus
	Metadata    *AssigneeMetadata
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type MigrationMappingRepository struct {
	db *sql.DB
}

func NewMigrationMappingRepository(db *sql.DB) *MigrationMappingRepository {
	return &MigrationMappingRepository{db: db}
}

func (r *MigrationMappingRepository) UpsertPending(
	migrationID int64,
	mappingType MappingType,
	sourceValue string,
	metadata *AssigneeMetadata,
) error {
	var metadataJSON *string
	if metadata != nil {
		b, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("marshal assignee metadata: %w", err)
		}
		s := string(b)
		metadataJSON = &s
	}

	query := `
		INSERT OR IGNORE INTO migration_mappings 
			(migration_id, type, source_value, status, metadata)
		VALUES (?, ?, ?, 'pending', ?)
	`

	_, err := r.db.Exec(query, migrationID, mappingType, sourceValue, metadataJSON)
	if err != nil {
		return fmt.Errorf("upsert pending mapping: %w", err)
	}
	return nil
}

func (r *MigrationMappingRepository) UpdateMapping(
	migrationID int64,
	mappingType MappingType,
	sourceValue string,
	destValue string,
) error {
	query := `
		UPDATE migration_mappings
		SET dest_value = ?, status = 'mapped', updated_at = CURRENT_TIMESTAMP
		WHERE migration_id = ? AND type = ? AND source_value = ?
	`

	result, err := r.db.Exec(query, destValue, migrationID, mappingType, sourceValue)
	if err != nil {
		return fmt.Errorf("update mapping: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update mapping rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("mapping not found: migration=%d type=%s source=%s", migrationID, mappingType, sourceValue)
	}
	return nil
}

func (r *MigrationMappingRepository) GetByMigrationID(
	migrationID int64,
	mappingType *MappingType,
) ([]MigrationMapping, error) {
	var rows *sql.Rows
	var err error

	if mappingType != nil {
		rows, err = r.db.Query(`
			SELECT id, migration_id, type, source_value, dest_value, status, metadata, created_at, updated_at
			FROM migration_mappings
			WHERE migration_id = ? AND type = ?
			ORDER BY created_at ASC
		`, migrationID, *mappingType)
	} else {
		rows, err = r.db.Query(`
			SELECT id, migration_id, type, source_value, dest_value, status, metadata, created_at, updated_at
			FROM migration_mappings
			WHERE migration_id = ?
			ORDER BY type ASC, created_at ASC
		`, migrationID)
	}

	if err != nil {
		return nil, fmt.Errorf("get mappings by migration id: %w", err)
	}
	defer rows.Close()

	var mappings []MigrationMapping
	for rows.Next() {
		var m MigrationMapping
		var destValue, metadataStr sql.NullString
		var mappingType string

		err := rows.Scan(
			&m.ID,
			&m.MigrationID,
			&mappingType,
			&m.SourceValue,
			&destValue,
			&m.Status,
			&metadataStr,
			&m.CreatedAt,
			&m.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan migration mapping: %w", err)
		}

		m.Type = MappingType(mappingType)

		if destValue.Valid {
			m.DestValue = &destValue.String
		}

		if metadataStr.Valid {
			var meta AssigneeMetadata
			if err := json.Unmarshal([]byte(metadataStr.String), &meta); err != nil {
				return nil, fmt.Errorf("parse assignee metadata: %w", err)
			}
			m.Metadata = &meta
		}

		mappings = append(mappings, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate migration mappings: %w", err)
	}

	return mappings, nil
}

func (r *MigrationMappingRepository) AllMapped(migrationID int64) (bool, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM migration_mappings
		WHERE migration_id = ? AND status = 'pending'
	`, migrationID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check all mapped: %w", err)
	}
	return count == 0, nil
}

func (r *MigrationMappingRepository) UpsertCustomField(
	migrationID int64, fieldID, fieldName, fieldType string,
) error {
	b, err := json.Marshal(CustomFieldMetadata{Name: fieldName, FieldType: fieldType})
	if err != nil {
		return fmt.Errorf("marshal custom field metadata: %w", err)
	}
	_, err = r.db.Exec(`
		INSERT OR IGNORE INTO migration_mappings
			(migration_id, type, source_value, status, metadata)
		VALUES (?, 'custom_field', ?, 'enabled', ?)
	`, migrationID, fieldID, string(b))
	if err != nil {
		return fmt.Errorf("upsert custom field: %w", err)
	}
	return nil
}

func (r *MigrationMappingRepository) UpdateCustomFieldEnabled(
	migrationID int64, fieldID string, enabled bool,
) error {
	status := MappingStatusEnabled
	if !enabled {
		status = MappingStatusDisabled
	}
	_, err := r.db.Exec(`
		UPDATE migration_mappings
		SET status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE migration_id = ? AND type = 'custom_field' AND source_value = ?
	`, status, migrationID, fieldID)
	if err != nil {
		return fmt.Errorf("update custom field enabled: %w", err)
	}
	return nil
}

func (r *MigrationMappingRepository) GetCustomFields(
	migrationID int64,
) ([]CustomFieldRow, error) {
	rows, err := r.db.Query(`
		SELECT source_value, status, metadata
		FROM migration_mappings
		WHERE migration_id = ? AND type = 'custom_field'
		ORDER BY created_at ASC
	`, migrationID)
	if err != nil {
		return nil, fmt.Errorf("get custom fields: %w", err)
	}
	defer rows.Close()

	var result []CustomFieldRow
	for rows.Next() {
		var fieldID, status string
		var metaStr sql.NullString
		if err := rows.Scan(&fieldID, &status, &metaStr); err != nil {
			return nil, fmt.Errorf("scan custom field: %w", err)
		}
		row := CustomFieldRow{
			FieldID: fieldID,
			Enabled: status == string(MappingStatusEnabled),
		}
		if metaStr.Valid {
			var meta CustomFieldMetadata
			if err := json.Unmarshal([]byte(metaStr.String), &meta); err == nil {
				row.FieldName = meta.Name
				row.FieldType = meta.FieldType
			}
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *MigrationMappingRepository) GetEnabledCustomFieldIDs(
	migrationID int64,
) (map[string]bool, error) {
	rows, err := r.db.Query(`
		SELECT source_value
		FROM migration_mappings
		WHERE migration_id = ? AND type = 'custom_field' AND status = 'enabled'
	`, migrationID)
	if err != nil {
		return nil, fmt.Errorf("get enabled custom field IDs: %w", err)
	}
	defer rows.Close()

	result := make(map[string]bool)
	for rows.Next() {
		var fieldID string
		if err := rows.Scan(&fieldID); err != nil {
			return nil, fmt.Errorf("scan custom field ID: %w", err)
		}
		result[fieldID] = true
	}
	return result, rows.Err()
}
