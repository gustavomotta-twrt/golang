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
	MappingStatusSkipped  MappingStatus = "skipped"
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
	ID                int64
	MigrationID       int64
	Type              MappingType
	SourceValue       string
	DestValue         *string
	SourceContainerID *string
	Status            MappingStatus
	Metadata          *AssigneeMetadata
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type MigrationMappingRepository struct {
	db *sql.DB
}

func NewMigrationMappingRepository(db *sql.DB) *MigrationMappingRepository {
	return &MigrationMappingRepository{db: db}
}

// UpsertPending inserts a new mapping row in 'pending' state if it does not already exist.
// sourceContainerID is nil for assignees (global), or the container ID for status/priority/custom_field.
func (r *MigrationMappingRepository) UpsertPending(
	migrationID int64,
	mappingType MappingType,
	sourceValue string,
	metadata *AssigneeMetadata,
	sourceContainerID *string,
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

	// For NULL container (assignees), we rely on the partial unique index to deduplicate.
	// INSERT OR IGNORE handles both the UNIQUE constraint and the partial index.
	query := `
		INSERT OR IGNORE INTO migration_mappings
			(migration_id, type, source_value, source_container_id, status, metadata)
		VALUES (?, ?, ?, ?, 'pending', ?)
	`
	_, err := r.db.Exec(query, migrationID, mappingType, sourceValue, sourceContainerID, metadataJSON)
	if err != nil {
		return fmt.Errorf("upsert pending mapping: %w", err)
	}
	return nil
}

// UpdateMapping sets dest_value and marks a specific mapping as 'mapped'.
// sourceContainerID nil targets global (NULL container) rows; non-nil targets per-container rows.
func (r *MigrationMappingRepository) UpdateMapping(
	migrationID int64,
	mappingType MappingType,
	sourceValue string,
	sourceContainerID *string,
	destValue string,
) error {
	var result sql.Result
	var err error

	if sourceContainerID == nil {
		result, err = r.db.Exec(`
			UPDATE migration_mappings
			SET dest_value = ?, status = 'mapped', updated_at = CURRENT_TIMESTAMP
			WHERE migration_id = ? AND type = ? AND source_value = ? AND source_container_id IS NULL
		`, destValue, migrationID, mappingType, sourceValue)
	} else {
		result, err = r.db.Exec(`
			UPDATE migration_mappings
			SET dest_value = ?, status = 'mapped', updated_at = CURRENT_TIMESTAMP
			WHERE migration_id = ? AND type = ? AND source_value = ? AND source_container_id = ?
		`, destValue, migrationID, mappingType, sourceValue, *sourceContainerID)
	}
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

// MarkContainerMappingsSkipped sets all status/priority rows for the given container to 'skipped'
// so they do not block AllMapped when the container is disabled.
func (r *MigrationMappingRepository) MarkContainerMappingsSkipped(migrationID int64, containerID string) error {
	_, err := r.db.Exec(`
		UPDATE migration_mappings
		SET status = 'skipped', updated_at = CURRENT_TIMESTAMP
		WHERE migration_id = ? AND source_container_id = ? AND type IN ('status', 'priority', 'custom_field')
	`, migrationID, containerID)
	if err != nil {
		return fmt.Errorf("mark container mappings skipped: %w", err)
	}
	return nil
}

// ReactivateContainerMappings resets 'skipped' rows back to 'pending' when a container is re-enabled.
func (r *MigrationMappingRepository) ReactivateContainerMappings(migrationID int64, containerID string) error {
	_, err := r.db.Exec(`
		UPDATE migration_mappings
		SET status = 'pending', dest_value = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE migration_id = ? AND source_container_id = ? AND status = 'skipped'
	`, migrationID, containerID)
	if err != nil {
		return fmt.Errorf("reactivate container mappings: %w", err)
	}
	return nil
}

// GetByMigrationIDAndContainer returns all mapping rows for a specific source container.
func (r *MigrationMappingRepository) GetByMigrationIDAndContainer(
	migrationID int64,
	containerID string,
) ([]MigrationMapping, error) {
	rows, err := r.db.Query(`
		SELECT id, migration_id, type, source_value, dest_value, source_container_id, status, metadata, created_at, updated_at
		FROM migration_mappings
		WHERE migration_id = ? AND source_container_id = ?
		ORDER BY type ASC, created_at ASC
	`, migrationID, containerID)
	if err != nil {
		return nil, fmt.Errorf("get mappings by container: %w", err)
	}
	defer rows.Close()
	return scanMappings(rows)
}

// GetGlobalByMigrationID returns all global (NULL source_container_id) mapping rows — i.e., assignees.
func (r *MigrationMappingRepository) GetGlobalByMigrationID(migrationID int64) ([]MigrationMapping, error) {
	rows, err := r.db.Query(`
		SELECT id, migration_id, type, source_value, dest_value, source_container_id, status, metadata, created_at, updated_at
		FROM migration_mappings
		WHERE migration_id = ? AND source_container_id IS NULL
		ORDER BY type ASC, created_at ASC
	`, migrationID)
	if err != nil {
		return nil, fmt.Errorf("get global mappings: %w", err)
	}
	defer rows.Close()
	return scanMappings(rows)
}

func scanMappings(rows *sql.Rows) ([]MigrationMapping, error) {
	var mappings []MigrationMapping
	for rows.Next() {
		var m MigrationMapping
		var destValue, containerID, metadataStr sql.NullString
		var mappingType string

		err := rows.Scan(
			&m.ID,
			&m.MigrationID,
			&mappingType,
			&m.SourceValue,
			&destValue,
			&containerID,
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
		if containerID.Valid {
			m.SourceContainerID = &containerID.String
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

// UpsertCustomField inserts a custom field mapping row for a specific container if it doesn't exist.
func (r *MigrationMappingRepository) UpsertCustomField(
	migrationID int64, fieldID, fieldName, fieldType string,
	sourceContainerID *string,
) error {
	b, err := json.Marshal(CustomFieldMetadata{Name: fieldName, FieldType: fieldType})
	if err != nil {
		return fmt.Errorf("marshal custom field metadata: %w", err)
	}
	_, err = r.db.Exec(`
		INSERT OR IGNORE INTO migration_mappings
			(migration_id, type, source_value, source_container_id, status, metadata)
		VALUES (?, 'custom_field', ?, ?, 'enabled', ?)
	`, migrationID, fieldID, sourceContainerID, string(b))
	if err != nil {
		return fmt.Errorf("upsert custom field: %w", err)
	}
	return nil
}

// UpdateCustomFieldEnabled updates the enabled/disabled status of a custom field for a specific container.
func (r *MigrationMappingRepository) UpdateCustomFieldEnabled(
	migrationID int64, fieldID string, enabled bool, sourceContainerID *string,
) error {
	status := MappingStatusEnabled
	if !enabled {
		status = MappingStatusDisabled
	}
	var err error
	if sourceContainerID == nil {
		_, err = r.db.Exec(`
			UPDATE migration_mappings
			SET status = ?, updated_at = CURRENT_TIMESTAMP
			WHERE migration_id = ? AND type = 'custom_field' AND source_value = ? AND source_container_id IS NULL
		`, status, migrationID, fieldID)
	} else {
		_, err = r.db.Exec(`
			UPDATE migration_mappings
			SET status = ?, updated_at = CURRENT_TIMESTAMP
			WHERE migration_id = ? AND type = 'custom_field' AND source_value = ? AND source_container_id = ?
		`, status, migrationID, fieldID, *sourceContainerID)
	}
	if err != nil {
		return fmt.Errorf("update custom field enabled: %w", err)
	}
	return nil
}

// GetCustomFields returns custom field rows for a specific container (or global if containerID is nil).
func (r *MigrationMappingRepository) GetCustomFields(
	migrationID int64, containerID *string,
) ([]CustomFieldRow, error) {
	var rows *sql.Rows
	var err error

	if containerID == nil {
		rows, err = r.db.Query(`
			SELECT source_value, status, metadata
			FROM migration_mappings
			WHERE migration_id = ? AND type = 'custom_field' AND source_container_id IS NULL
			ORDER BY created_at ASC
		`, migrationID)
	} else {
		rows, err = r.db.Query(`
			SELECT source_value, status, metadata
			FROM migration_mappings
			WHERE migration_id = ? AND type = 'custom_field' AND source_container_id = ?
			ORDER BY created_at ASC
		`, migrationID, *containerID)
	}
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

// GetEnabledCustomFieldIDs returns all distinct enabled custom field IDs for a migration,
// across all containers (used during execution to build the custom field mapping).
func (r *MigrationMappingRepository) GetEnabledCustomFieldIDs(
	migrationID int64,
) (map[string]bool, error) {
	rows, err := r.db.Query(`
		SELECT DISTINCT source_value
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
