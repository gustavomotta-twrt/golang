package repository

import (
	"database/sql"
	"fmt"
)

type ContainerMapping struct {
	ID         int64
	MigrationID int64
	SourceID   string
	SourceName string
	DestID     *string
	DestName   *string
	Status     string
}

type ContainerMappingRepository struct {
	db *sql.DB
}

func NewContainerMappingRepository(db *sql.DB) *ContainerMappingRepository {
	return &ContainerMappingRepository{db: db}
}

func (r *ContainerMappingRepository) Upsert(migrationID int64, sourceID, sourceName string) error {
	_, err := r.db.Exec(`
		INSERT OR IGNORE INTO container_mappings (migration_id, source_id, source_name, status)
		VALUES (?, ?, ?, 'pending')
	`, migrationID, sourceID, sourceName)
	if err != nil {
		return fmt.Errorf("upsert container mapping: %w", err)
	}
	return nil
}

func (r *ContainerMappingRepository) UpdateMapping(migrationID int64, sourceID, destID, destName string) error {
	result, err := r.db.Exec(`
		UPDATE container_mappings
		SET dest_id = ?, dest_name = ?, status = 'mapped'
		WHERE migration_id = ? AND source_id = ?
	`, destID, destName, migrationID, sourceID)
	if err != nil {
		return fmt.Errorf("update container mapping: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update container mapping rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("container mapping not found: migration=%d source=%s", migrationID, sourceID)
	}
	return nil
}

func (r *ContainerMappingRepository) GetByMigrationID(migrationID int64) ([]ContainerMapping, error) {
	rows, err := r.db.Query(`
		SELECT id, migration_id, source_id, source_name, dest_id, dest_name, status
		FROM container_mappings
		WHERE migration_id = ?
		ORDER BY id ASC
	`, migrationID)
	if err != nil {
		return nil, fmt.Errorf("get container mappings: %w", err)
	}
	defer rows.Close()

	var mappings []ContainerMapping
	for rows.Next() {
		var m ContainerMapping
		var destID, destName sql.NullString
		if err := rows.Scan(&m.ID, &m.MigrationID, &m.SourceID, &m.SourceName, &destID, &destName, &m.Status); err != nil {
			return nil, fmt.Errorf("scan container mapping: %w", err)
		}
		if destID.Valid {
			m.DestID = &destID.String
		}
		if destName.Valid {
			m.DestName = &destName.String
		}
		mappings = append(mappings, m)
	}
	return mappings, rows.Err()
}

func (r *ContainerMappingRepository) AllMapped(migrationID int64) (bool, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM container_mappings
		WHERE migration_id = ? AND status = 'pending'
	`, migrationID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check all containers mapped: %w", err)
	}
	return count == 0, nil
}
