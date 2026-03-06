package service

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/TWRT/integration-mapper/internal/client"
	"github.com/TWRT/integration-mapper/internal/models"
	"github.com/TWRT/integration-mapper/internal/repository"
)

// Internal repository interfaces — allow mock injection in tests.

type migrationRepo interface {
	Create(migration *repository.Migration) (int64, error)
	UpdateProgress(id int64, completed, failed int) error
	UpdateStatus(id int64, status repository.MigrationStatus) error
	Complete(id int64, status repository.MigrationStatus) error
	UpdateTotalTasks(id int64, totalTasks int) error
	GetMigration(id int64) (repository.Migration, error)
	GetMigrations() ([]repository.Migration, error)
}

type taskMappingRepo interface {
	Create(mapping *repository.TaskMapping) error
}

type migrationMappingRepo interface {
	UpsertPending(migrationID int64, mappingType repository.MappingType, sourceValue string, metadata *repository.AssigneeMetadata, sourceContainerID *string) error
	UpdateMapping(migrationID int64, mappingType repository.MappingType, sourceValue string, sourceContainerID *string, destValue string) error
	MarkContainerMappingsSkipped(migrationID int64, containerID string) error
	ReactivateContainerMappings(migrationID int64, containerID string) error
	GetByMigrationIDAndContainer(migrationID int64, containerID string) ([]repository.MigrationMapping, error)
	GetGlobalByMigrationID(migrationID int64) ([]repository.MigrationMapping, error)
	AllMapped(migrationID int64) (bool, error)
	UpsertCustomField(migrationID int64, fieldID, fieldName, fieldType string, sourceContainerID *string) error
	UpdateCustomFieldEnabled(migrationID int64, fieldID string, enabled bool, sourceContainerID *string) error
	GetCustomFields(migrationID int64, containerID *string) ([]repository.CustomFieldRow, error)
	GetEnabledCustomFieldIDs(migrationID int64) (map[string]bool, error)
}

type containerMappingRepo interface {
	Upsert(migrationID int64, sourceID, sourceName string) error
	UpdateMapping(migrationID int64, sourceID, destID, destName string, enabled bool) error
	GetByMigrationID(migrationID int64) ([]repository.ContainerMapping, error)
	AllMapped(migrationID int64) (bool, error)
}

type MigrationService struct {
	providers            map[string]client.IntegrationProvider
	migrationRepo        migrationRepo
	taskMappingRepo      taskMappingRepo
	migrationMappingRepo migrationMappingRepo
	containerMappingRepo containerMappingRepo
}

func NewMigrationService(
	providers map[string]client.IntegrationProvider,
	migrationRepo migrationRepo,
	taskMappingRepo taskMappingRepo,
	migrationMappingRepo migrationMappingRepo,
	containerMappingRepo containerMappingRepo,
) *MigrationService {
	return &MigrationService{
		providers:            providers,
		migrationRepo:        migrationRepo,
		taskMappingRepo:      taskMappingRepo,
		migrationMappingRepo: migrationMappingRepo,
		containerMappingRepo: containerMappingRepo,
	}
}

// MigrationServiceProvider is the interface consumed by handlers.
// Allows substitution with mocks in tests.
type MigrationServiceProvider interface {
	CreateMigration(ctx context.Context, input CreateMigrationInput) (int64, *MappingsState, error)
	SyncMappings(ctx context.Context, migrationID int64) (*MappingsState, error)
	SaveMappings(ctx context.Context, migrationID int64, assignees []AssigneeMappingInput, containerMappings []ContainerMappingInput) (*MappingsState, error)
	GetDestContainerOptions(ctx context.Context, migrationID int64, destContainerID string) (statuses []string, priorities []string, err error)
	StartMigration(migrationID int64) error
	GetMigration(id int64) (repository.Migration, error)
	GetMigrations() ([]repository.Migration, error)
}

// ---- Types ----

type MappingItem struct {
	SourceValue string
	DestValue   *string
	Status      string
}

type FieldMappingInput struct {
	SourceValue string
	DestValue   string
}

type AssigneeMappingItem struct {
	MappingItem
	Name  string
	Email string
}

type AssigneeMappingInput struct {
	SourceValue string
	DestValue   string
}

type CustomFieldState struct {
	ID      string
	Name    string
	Type    string
	Enabled bool
}

type CustomFieldSelection struct {
	FieldID string
	Enabled bool
}

type AvailableContainer struct {
	ID   string
	Name string
}

// ContainerMappingDetail holds the full state for a single source container accordion.
type ContainerMappingDetail struct {
	SourceID                string
	SourceName              string
	DestID                  *string
	DestName                *string
	Enabled                 bool
	Status                  string
	StatusMappings          []MappingItem
	PriorityMappings        []MappingItem
	CustomFields            []CustomFieldState
	AvailableDestStatuses   []string
	AvailableDestPriorities []string
}

// ContainerMappingInput is the save payload for a single source container.
type ContainerMappingInput struct {
	SourceID         string
	DestID           *string
	DestName         *string
	Enabled          bool
	StatusMappings   []FieldMappingInput
	PriorityMappings []FieldMappingInput
	CustomFields     []CustomFieldSelection
}

// MappingsState is the full mappings state returned to the frontend.
type MappingsState struct {
	Assignees               []AssigneeMappingItem
	AvailableDestMembers    []models.Member
	ContainerMappings       []ContainerMappingDetail
	AvailableDestContainers []AvailableContainer
}

// ---- Provider helpers ----

func (s *MigrationService) getProvider(name string) (client.IntegrationProvider, error) {
	p, ok := s.providers[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
	return p, nil
}

func (s *MigrationService) getMembersForDestination(ctx context.Context, destination, destWorkspaceId string) ([]models.Member, error) {
	if destWorkspaceId == "" {
		return nil, fmt.Errorf("dest_workspace_id is required for destination %s", destination)
	}
	p, err := s.getProvider(destination)
	if err != nil {
		return nil, err
	}
	return p.GetMembers(ctx, destWorkspaceId)
}

func (s *MigrationService) getAvailableDestStatuses(ctx context.Context, destination, destListId string) ([]string, error) {
	p, err := s.getProvider(destination)
	if err != nil {
		return nil, err
	}
	return p.GetListStatuses(ctx, destListId)
}

func (s *MigrationService) getAvailableDestPrioritiesForState(ctx context.Context, destination, destListID string) []string {
	provider, err := s.getProvider(destination)
	if err != nil {
		return getAvailableDestPriorities(destination)
	}
	lookup, ok := provider.(client.PriorityLookup)
	if !ok {
		return getAvailableDestPriorities(destination)
	}
	options, err := lookup.GetProjectCustomFieldOptions(ctx, destListID)
	if err != nil || len(options) == 0 {
		return getAvailableDestPriorities(destination)
	}
	names := make([]string, 0, len(options))
	for k := range options {
		if k != "__field_gid__" {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	return names
}

func getAvailableDestPriorities(destination string) []string {
	if destination == "clickup" {
		return []string{"urgent", "high", "normal", "low"}
	}
	return []string{"High", "Medium", "Low"}
}

func (s *MigrationService) getAvailableDestContainers(ctx context.Context, migration repository.Migration) []AvailableContainer {
	destProvider, err := s.getProvider(migration.Destination)
	if err != nil {
		return nil
	}
	cp, ok := destProvider.(client.ContainerProvider)
	if !ok {
		return nil
	}
	destContainerID := s.getDestContainerID(migration)
	destContainers, err := cp.GetDestContainers(ctx, destContainerID)
	if err != nil {
		slog.Warn("could not fetch dest containers", "error", err)
		return nil
	}
	available := make([]AvailableContainer, len(destContainers))
	for i, dc := range destContainers {
		available[i] = AvailableContainer{ID: dc.ID, Name: dc.Name}
	}
	return available
}

func (s *MigrationService) getDestContainerID(migration repository.Migration) string {
	if migration.DestSpaceID != "" {
		return migration.DestSpaceID
	}
	return migration.DestListID
}

// ---- Custom field discovery ----

type containerCustomField struct {
	ContainerID string
	Def         models.CustomFieldDefinition
}

func (s *MigrationService) discoverCustomFieldsPerContainer(
	ctx context.Context,
	sourceClient client.IntegrationProvider,
	sourceProjectID string,
) []containerCustomField {
	fp, ok := sourceClient.(client.FieldProvider)
	if !ok {
		return nil
	}

	cp, hasCp := sourceClient.(client.ContainerProvider)
	if hasCp {
		containers, err := cp.GetSourceContainers(ctx, sourceProjectID)
		if err != nil || len(containers) == 0 {
			// Fall back to project-level
			defs, _ := fp.GetFieldDefinitions(ctx, sourceProjectID)
			return defsToContainerFields(sourceProjectID, defs)
		}
		var result []containerCustomField
		for _, c := range containers {
			defs, err := fp.GetFieldDefinitions(ctx, c.ID)
			if err != nil {
				slog.Warn("could not discover custom fields for container", "container", c.Name, "error", err)
				continue
			}
			result = append(result, defsToContainerFields(c.ID, defs)...)
		}
		return result
	}

	defs, _ := fp.GetFieldDefinitions(ctx, sourceProjectID)
	return defsToContainerFields(sourceProjectID, defs)
}

func defsToContainerFields(containerID string, defs []models.CustomFieldDefinition) []containerCustomField {
	result := make([]containerCustomField, len(defs))
	for i, d := range defs {
		result[i] = containerCustomField{ContainerID: containerID, Def: d}
	}
	return result
}

// ---- Mapping discovery ----

// discoverAndUpsertMappingsFromContainers fetches tasks per container and stores
// status/priority mappings with their source_container_id. Assignees are global (NULL container).
func (s *MigrationService) discoverAndUpsertMappingsFromContainers(
	ctx context.Context,
	migrationID int64,
	sourceProvider client.IntegrationProvider,
	sourceID string,
) ([]models.Task, error) {
	cp, hasContainers := sourceProvider.(client.ContainerProvider)

	var allTasks []models.Task
	globalAssignees := make(map[string]models.TaskAssignee)

	if hasContainers {
		containers, err := cp.GetSourceContainers(ctx, sourceID)
		if err != nil {
			return nil, fmt.Errorf("get source containers for mapping discovery: %w", err)
		}
		for _, c := range containers {
			tasks, err := cp.GetTasksByContainer(ctx, c.ID)
			if err != nil {
				slog.Warn("could not get tasks for container, skipping", "container", c.Name, "error", err)
				continue
			}

			uniqueStatuses := make(map[string]struct{})
			uniquePriorities := make(map[string]struct{})
			for _, task := range tasks {
				if task.Status != "" {
					uniqueStatuses[task.Status] = struct{}{}
				}
				if task.Priority != "" {
					uniquePriorities[task.Priority] = struct{}{}
				}
				for _, a := range task.Assignees {
					if a.ID != "" {
						globalAssignees[a.ID] = a
					}
				}
			}

			containerID := c.ID
			for status := range uniqueStatuses {
				if err := s.migrationMappingRepo.UpsertPending(migrationID, repository.MappingTypeStatus, status, nil, &containerID); err != nil {
					return nil, fmt.Errorf("upsert status mapping: %w", err)
				}
			}
			for priority := range uniquePriorities {
				if err := s.migrationMappingRepo.UpsertPending(migrationID, repository.MappingTypePriority, priority, nil, &containerID); err != nil {
					return nil, fmt.Errorf("upsert priority mapping: %w", err)
				}
			}

			allTasks = append(allTasks, tasks...)
		}
	} else {
		tasks, err := sourceProvider.GetTasks(ctx, sourceID)
		if err != nil {
			return nil, fmt.Errorf("get tasks from source: %w", err)
		}
		uniqueStatuses := make(map[string]struct{})
		uniquePriorities := make(map[string]struct{})
		for _, task := range tasks {
			if task.Status != "" {
				uniqueStatuses[task.Status] = struct{}{}
			}
			if task.Priority != "" {
				uniquePriorities[task.Priority] = struct{}{}
			}
			for _, a := range task.Assignees {
				if a.ID != "" {
					globalAssignees[a.ID] = a
				}
			}
		}
		for status := range uniqueStatuses {
			if err := s.migrationMappingRepo.UpsertPending(migrationID, repository.MappingTypeStatus, status, nil, nil); err != nil {
				return nil, fmt.Errorf("upsert status mapping: %w", err)
			}
		}
		for priority := range uniquePriorities {
			if err := s.migrationMappingRepo.UpsertPending(migrationID, repository.MappingTypePriority, priority, nil, nil); err != nil {
				return nil, fmt.Errorf("upsert priority mapping: %w", err)
			}
		}
		allTasks = tasks
	}

	// Insert assignees globally (NULL container).
	for _, assignee := range globalAssignees {
		metadata := &repository.AssigneeMetadata{Name: assignee.Name, Email: assignee.Email}
		if err := s.migrationMappingRepo.UpsertPending(migrationID, repository.MappingTypeAssignee, assignee.ID, metadata, nil); err != nil {
			return nil, fmt.Errorf("upsert assignee mapping: %w", err)
		}
	}

	return allTasks, nil
}

// ---- State building ----

func (s *MigrationService) loadCustomFieldsStateForContainer(migrationID int64, containerID string) []CustomFieldState {
	dbRows, err := s.migrationMappingRepo.GetCustomFields(migrationID, &containerID)
	if err != nil {
		return []CustomFieldState{}
	}
	result := make([]CustomFieldState, 0, len(dbRows))
	for _, r := range dbRows {
		result = append(result, CustomFieldState{
			ID:      r.FieldID,
			Name:    r.FieldName,
			Type:    r.FieldType,
			Enabled: r.Enabled,
		})
	}
	return result
}

func (s *MigrationService) buildMappingsState(ctx context.Context, migration repository.Migration) (*MappingsState, error) {
	// Global mappings (assignees, NULL container)
	globalMappings, err := s.migrationMappingRepo.GetGlobalByMigrationID(migration.Id)
	if err != nil {
		return nil, fmt.Errorf("get global mappings: %w", err)
	}

	destMembers, err := s.getMembersForDestination(ctx, migration.Destination, migration.DestWorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("get destination members: %w", err)
	}

	var assignees []AssigneeMappingItem
	for _, m := range globalMappings {
		if m.Type != repository.MappingTypeAssignee {
			continue
		}
		item := AssigneeMappingItem{
			MappingItem: MappingItem{
				SourceValue: m.SourceValue,
				DestValue:   m.DestValue,
				Status:      string(m.Status),
			},
		}
		if m.Metadata != nil {
			item.Name = m.Metadata.Name
			item.Email = m.Metadata.Email
		}
		assignees = append(assignees, item)
	}

	// Per-container details
	containerMappings, err := s.containerMappingRepo.GetByMigrationID(migration.Id)
	if err != nil {
		return nil, fmt.Errorf("get container mappings: %w", err)
	}

	availableContainers := s.getAvailableDestContainers(ctx, migration)

	containerDetails := make([]ContainerMappingDetail, 0, len(containerMappings))
	for _, cm := range containerMappings {
		detail := ContainerMappingDetail{
			SourceID:   cm.SourceID,
			SourceName: cm.SourceName,
			DestID:     cm.DestID,
			DestName:   cm.DestName,
			Enabled:    cm.Enabled,
			Status:     cm.Status,
		}

		// Load per-container status/priority
		perContainerMappings, err := s.migrationMappingRepo.GetByMigrationIDAndContainer(migration.Id, cm.SourceID)
		if err != nil {
			slog.Warn("could not load per-container mappings", "container", cm.SourceID, "error", err)
		}
		for _, m := range perContainerMappings {
			item := MappingItem{
				SourceValue: m.SourceValue,
				DestValue:   m.DestValue,
				Status:      string(m.Status),
			}
			switch m.Type {
			case repository.MappingTypeStatus:
				detail.StatusMappings = append(detail.StatusMappings, item)
			case repository.MappingTypePriority:
				detail.PriorityMappings = append(detail.PriorityMappings, item)
			}
		}

		// Load per-container custom fields
		detail.CustomFields = s.loadCustomFieldsStateForContainer(migration.Id, cm.SourceID)

		// Fetch available dest options if destination is set
		if cm.DestID != nil {
			if ss, err := s.getAvailableDestStatuses(ctx, migration.Destination, *cm.DestID); err == nil {
				detail.AvailableDestStatuses = ss
			} else {
				slog.Warn("could not fetch dest statuses for container", "destID", *cm.DestID, "error", err)
			}
			detail.AvailableDestPriorities = s.getAvailableDestPrioritiesForState(ctx, migration.Destination, *cm.DestID)
		}

		containerDetails = append(containerDetails, detail)
	}

	return &MappingsState{
		Assignees:               assignees,
		AvailableDestMembers:    destMembers,
		ContainerMappings:       containerDetails,
		AvailableDestContainers: availableContainers,
	}, nil
}

// ---- Service operations ----

type CreateMigrationInput struct {
	Source          string
	Destination     string
	SourceProjectID string
	DestListID      string
	DestWorkspaceID string
	DestSpaceID     string
}

func (s *MigrationService) CreateMigration(ctx context.Context, input CreateMigrationInput) (int64, *MappingsState, error) {
	migration := &repository.Migration{
		Source:          input.Source,
		Destination:     input.Destination,
		SourceProjectID: input.SourceProjectID,
		DestListID:      input.DestListID,
		DestWorkspaceID: input.DestWorkspaceID,
		DestSpaceID:     input.DestSpaceID,
		Status:          repository.MigrationStatusPendingConfiguration,
	}

	migrationID, err := s.migrationRepo.Create(migration)
	if err != nil {
		return 0, nil, fmt.Errorf("create migration: %w", err)
	}

	sourceProvider, err := s.getProvider(input.Source)
	if err != nil {
		return 0, nil, fmt.Errorf("get source provider: %w", err)
	}

	// Discover source containers
	if cp, ok := sourceProvider.(client.ContainerProvider); ok {
		containers, err := cp.GetSourceContainers(ctx, input.SourceProjectID)
		if err != nil {
			return 0, nil, fmt.Errorf("discover source containers: %w", err)
		}
		for _, c := range containers {
			if err := s.containerMappingRepo.Upsert(migrationID, c.ID, c.Name); err != nil {
				return 0, nil, fmt.Errorf("upsert container mapping: %w", err)
			}
		}
	}

	if _, err := s.discoverAndUpsertMappingsFromContainers(ctx, migrationID, sourceProvider, input.SourceProjectID); err != nil {
		return 0, nil, fmt.Errorf("discover mappings: %w", err)
	}

	for _, cf := range s.discoverCustomFieldsPerContainer(ctx, sourceProvider, input.SourceProjectID) {
		containerID := cf.ContainerID
		if err := s.migrationMappingRepo.UpsertCustomField(migrationID, cf.Def.ID, cf.Def.Name, cf.Def.ClickUpType, &containerID); err != nil {
			slog.Warn("could not persist custom field", "field", cf.Def.Name, "error", err)
		}
	}

	migration.Id = migrationID
	state, err := s.buildMappingsState(ctx, *migration)
	if err != nil {
		return 0, nil, fmt.Errorf("build mappings state: %w", err)
	}

	return migrationID, state, nil
}

func (s *MigrationService) SyncMappings(ctx context.Context, migrationID int64) (*MappingsState, error) {
	migration, err := s.migrationRepo.GetMigration(migrationID)
	if err != nil {
		return nil, fmt.Errorf("get migration: %w", err)
	}

	sourceProvider, err := s.getProvider(migration.Source)
	if err != nil {
		return nil, fmt.Errorf("get source provider: %w", err)
	}

	if cp, ok := sourceProvider.(client.ContainerProvider); ok {
		containers, err := cp.GetSourceContainers(ctx, migration.SourceProjectID)
		if err != nil {
			return nil, fmt.Errorf("sync containers: %w", err)
		}
		for _, c := range containers {
			if err := s.containerMappingRepo.Upsert(migrationID, c.ID, c.Name); err != nil {
				slog.Warn("could not upsert container on sync", "container", c.Name, "error", err)
			}
		}
	}

	if _, err := s.discoverAndUpsertMappingsFromContainers(ctx, migrationID, sourceProvider, migration.SourceProjectID); err != nil {
		return nil, fmt.Errorf("sync mappings: %w", err)
	}

	for _, cf := range s.discoverCustomFieldsPerContainer(ctx, sourceProvider, migration.SourceProjectID) {
		containerID := cf.ContainerID
		if err := s.migrationMappingRepo.UpsertCustomField(migrationID, cf.Def.ID, cf.Def.Name, cf.Def.ClickUpType, &containerID); err != nil {
			slog.Warn("could not persist custom field on sync", "field", cf.Def.Name, "error", err)
		}
	}

	return s.buildMappingsState(ctx, migration)
}

func (s *MigrationService) SaveMappings(
	ctx context.Context,
	migrationID int64,
	assignees []AssigneeMappingInput,
	containerMappings []ContainerMappingInput,
) (*MappingsState, error) {
	migration, err := s.migrationRepo.GetMigration(migrationID)
	if err != nil {
		return nil, fmt.Errorf("get migration: %w", err)
	}

	// Save global assignee mappings
	for _, a := range assignees {
		if a.DestValue == "" {
			continue
		}
		if err := s.migrationMappingRepo.UpdateMapping(migrationID, repository.MappingTypeAssignee, a.SourceValue, nil, a.DestValue); err != nil {
			slog.Warn("could not save assignee mapping", "source", a.SourceValue, "error", err)
		}
	}

	// Save per-container mappings
	for _, cm := range containerMappings {
		destID := ""
		destName := ""
		if cm.DestID != nil {
			destID = *cm.DestID
		}
		if cm.DestName != nil {
			destName = *cm.DestName
		}
		if err := s.containerMappingRepo.UpdateMapping(migrationID, cm.SourceID, destID, destName, cm.Enabled); err != nil {
			return nil, fmt.Errorf("save container mapping %s: %w", cm.SourceID, err)
		}

		if !cm.Enabled {
			// Mark all this container's field mappings as skipped so they don't block AllMapped.
			if err := s.migrationMappingRepo.MarkContainerMappingsSkipped(migrationID, cm.SourceID); err != nil {
				slog.Warn("could not mark container mappings skipped", "container", cm.SourceID, "error", err)
			}
			continue
		}

		// Re-activate if previously skipped
		if err := s.migrationMappingRepo.ReactivateContainerMappings(migrationID, cm.SourceID); err != nil {
			slog.Warn("could not reactivate container mappings", "container", cm.SourceID, "error", err)
		}

		for _, sm := range cm.StatusMappings {
			if sm.DestValue == "" {
				continue
			}
			if err := s.migrationMappingRepo.UpdateMapping(migrationID, repository.MappingTypeStatus, sm.SourceValue, &cm.SourceID, sm.DestValue); err != nil {
				slog.Warn("could not save status mapping", "container", cm.SourceID, "source", sm.SourceValue, "error", err)
			}
		}

		for _, pm := range cm.PriorityMappings {
			if pm.DestValue == "" {
				continue
			}
			if err := s.migrationMappingRepo.UpdateMapping(migrationID, repository.MappingTypePriority, pm.SourceValue, &cm.SourceID, pm.DestValue); err != nil {
				slog.Warn("could not save priority mapping", "container", cm.SourceID, "source", pm.SourceValue, "error", err)
			}
		}

		for _, cf := range cm.CustomFields {
			if err := s.migrationMappingRepo.UpdateCustomFieldEnabled(migrationID, cf.FieldID, cf.Enabled, &cm.SourceID); err != nil {
				slog.Warn("could not save custom field selection", "container", cm.SourceID, "field", cf.FieldID, "error", err)
			}
		}
	}

	fieldsMapped, err := s.migrationMappingRepo.AllMapped(migrationID)
	if err != nil {
		return nil, fmt.Errorf("check all field mappings done: %w", err)
	}
	containersMapped, err := s.containerMappingRepo.AllMapped(migrationID)
	if err != nil {
		return nil, fmt.Errorf("check all container mappings done: %w", err)
	}

	if fieldsMapped && containersMapped {
		if err := s.migrationRepo.UpdateStatus(migrationID, repository.MigrationStatusReadyToStart); err != nil {
			return nil, fmt.Errorf("update migration status: %w", err)
		}
	} else {
		if err := s.migrationRepo.UpdateStatus(migrationID, repository.MigrationStatusPendingConfiguration); err != nil {
			return nil, fmt.Errorf("update migration status: %w", err)
		}
	}

	return s.buildMappingsState(ctx, migration)
}

// GetDestContainerOptions returns the available statuses and priorities for a specific destination container.
func (s *MigrationService) GetDestContainerOptions(ctx context.Context, migrationID int64, destContainerID string) (statuses []string, priorities []string, err error) {
	migration, err := s.migrationRepo.GetMigration(migrationID)
	if err != nil {
		return nil, nil, fmt.Errorf("get migration: %w", err)
	}

	statuses, err = s.getAvailableDestStatuses(ctx, migration.Destination, destContainerID)
	if err != nil {
		return nil, nil, fmt.Errorf("get dest statuses: %w", err)
	}

	priorities = s.getAvailableDestPrioritiesForState(ctx, migration.Destination, destContainerID)
	return statuses, priorities, nil
}

func (s *MigrationService) StartMigration(migrationID int64) error {
	allMapped, err := s.migrationMappingRepo.AllMapped(migrationID)
	if err != nil {
		return fmt.Errorf("check mappings: %w", err)
	}
	if !allMapped {
		return fmt.Errorf("there are pending field mappings — configure all before starting")
	}

	containersMapped, err := s.containerMappingRepo.AllMapped(migrationID)
	if err != nil {
		return fmt.Errorf("check container mappings: %w", err)
	}
	if !containersMapped {
		return fmt.Errorf("there are unmapped sections/lists — map all containers before starting")
	}

	migration, err := s.migrationRepo.GetMigration(migrationID)
	if err != nil {
		return fmt.Errorf("get migration: %w", err)
	}

	sourceProvider, err := s.getProvider(migration.Source)
	if err != nil {
		return fmt.Errorf("get source provider: %w", err)
	}
	destProvider, err := s.getProvider(migration.Destination)
	if err != nil {
		return fmt.Errorf("get dest provider: %w", err)
	}
	if err := s.migrationRepo.UpdateStatus(migrationID, repository.MigrationStatusRunning); err != nil {
		return fmt.Errorf("update migration status: %w", err)
	}

	// Create an independent context — not tied to the HTTP request lifecycle.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)

	go func() {
		defer cancel()
		s.executeMigration(ctx, sourceProvider, destProvider, migration)
	}()

	return nil
}

// ---- Execution ----

func mapStatus(taskStatus string, mappings []MappingItem) string {
	for _, m := range mappings {
		if m.SourceValue == taskStatus && m.DestValue != nil {
			return *m.DestValue
		}
	}
	return "to do"
}

func mapPriority(taskPriority string, mappings []MappingItem) string {
	if taskPriority == "" {
		return ""
	}
	for _, m := range mappings {
		if m.SourceValue == taskPriority && m.DestValue != nil {
			return *m.DestValue
		}
	}
	return ""
}

func (s *MigrationService) executeMigration(
	ctx context.Context,
	sourceClient client.TaskClient,
	destClient client.TaskClient,
	migration repository.Migration,
) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in executeMigration",
				"migration_id", migration.Id,
				"panic", r,
				"stack", string(debug.Stack()),
			)
			s.migrationRepo.Complete(migration.Id, repository.MigrationStatusFailed)
		}
	}()

	containerMappings, err := s.containerMappingRepo.GetByMigrationID(migration.Id)
	if err != nil {
		s.migrationRepo.Complete(migration.Id, repository.MigrationStatusFailed)
		slog.Error("failed to load container mappings", "migration_id", migration.Id, "error", err)
		return
	}

	// Load global assignee mappings (NULL container)
	globalMappings, err := s.migrationMappingRepo.GetGlobalByMigrationID(migration.Id)
	if err != nil {
		s.migrationRepo.Complete(migration.Id, repository.MigrationStatusFailed)
		slog.Error("failed to load global mappings", "migration_id", migration.Id, "error", err)
		return
	}
	resolvedAssignees := make(map[string]string)
	for _, m := range globalMappings {
		if m.Type == repository.MappingTypeAssignee && m.DestValue != nil {
			resolvedAssignees[m.SourceValue] = *m.DestValue
		}
	}

	// Build custom field mapping (global across all containers for execution simplicity)
	cfMapping := map[string]customFieldEntry{}
	if fp, ok := sourceClient.(client.FieldProvider); ok {
		if fc, ok := destClient.(client.FieldCreator); ok {
			sourceProvider, _ := sourceClient.(client.IntegrationProvider)
			listIDs := s.resolveListIDs(ctx, sourceProvider, migration.SourceProjectID)
			cfMapping = s.buildCustomFieldMapping(ctx, fp, fc, listIDs, migration.DestWorkspaceID, migration.DestListID)
			enabledIDs, err := s.migrationMappingRepo.GetEnabledCustomFieldIDs(migration.Id)
			if err != nil {
				slog.Warn("could not load enabled custom field IDs, migrating all fields", "migration_id", migration.Id, "error", err)
			} else {
				for id := range cfMapping {
					if !enabledIDs[id] {
						delete(cfMapping, id)
					}
				}
			}
		}
	}

	// Priority options (for Asana destination)
	priorityOptions := map[string]string{}
	if lookup, ok := destClient.(client.PriorityLookup); ok {
		options, err := lookup.GetProjectCustomFieldOptions(ctx, migration.DestListID)
		if err != nil {
			s.migrationRepo.Complete(migration.Id, repository.MigrationStatusFailed)
			slog.Error("failed to fetch priority options", "migration_id", migration.Id, "error", err)
			return
		}
		priorityOptions = options
	}

	cp, hasContainerProvider := sourceClient.(client.ContainerProvider)

	successCount := 0
	failCount := 0
	totalTasks := 0

	if hasContainerProvider && len(containerMappings) > 0 {
		// First pass: count total tasks
		var tasksByContainer []struct {
			destID string
			tasks  []models.Task
			status []MappingItem
			prio   []MappingItem
		}

		for _, cm := range containerMappings {
			if !cm.Enabled {
				slog.Info("container disabled, skipping", "source_container", cm.SourceName)
				continue
			}
			if cm.DestID == nil {
				slog.Warn("container has no dest mapping, skipping", "source_container", cm.SourceName)
				continue
			}

			containerTasks, err := cp.GetTasksByContainer(ctx, cm.SourceID)
			if err != nil {
				s.migrationRepo.Complete(migration.Id, repository.MigrationStatusFailed)
				slog.Error("failed to fetch tasks for container", "container", cm.SourceName, "error", err)
				return
			}

			// Load per-container status/priority mappings
			perContainerMappings, err := s.migrationMappingRepo.GetByMigrationIDAndContainer(migration.Id, cm.SourceID)
			if err != nil {
				slog.Warn("could not load per-container mappings, using empty", "container", cm.SourceID, "error", err)
			}
			var statusMappings, priorityMappings []MappingItem
			for _, m := range perContainerMappings {
				item := MappingItem{SourceValue: m.SourceValue, DestValue: m.DestValue, Status: string(m.Status)}
				switch m.Type {
				case repository.MappingTypeStatus:
					statusMappings = append(statusMappings, item)
				case repository.MappingTypePriority:
					priorityMappings = append(priorityMappings, item)
				}
			}

			destID := *cm.DestID
			if migration.Destination == "asana" {
				destID = migration.DestListID + "|" + *cm.DestID
			}

			tasksByContainer = append(tasksByContainer, struct {
				destID string
				tasks  []models.Task
				status []MappingItem
				prio   []MappingItem
			}{destID, containerTasks, statusMappings, priorityMappings})

			totalTasks += len(containerTasks)
		}

		s.migrationRepo.UpdateTotalTasks(migration.Id, totalTasks)

		slog.Info("starting migration",
			"migration_id", migration.Id,
			"source", migration.Source,
			"destination", migration.Destination,
			"total_tasks", totalTasks,
			"custom_fields_mapped", len(cfMapping),
		)

		for _, group := range tasksByContainer {
			for _, task := range group.tasks {
				slog.Info("migrating task", "migration_id", migration.Id, "task_id", task.Id, "task_name", task.Name)

				task.Status = mapStatus(task.Status, group.status)
				task.Priority = mapPriority(task.Priority, group.prio)

				if task.Priority != "" && len(priorityOptions) > 0 {
					fieldGid := priorityOptions["__field_gid__"]
					optionGid := priorityOptions[task.Priority]
					if fieldGid != "" && optionGid != "" {
						task.Priority = fieldGid + ":" + optionGid
					} else {
						task.Priority = ""
					}
				}

				destAssignees := make([]models.TaskAssignee, 0, len(task.Assignees))
				for _, a := range task.Assignees {
					if destID, ok := resolvedAssignees[a.ID]; ok {
						destAssignees = append(destAssignees, models.TaskAssignee{ID: destID})
					}
				}
				task.Assignees = destAssignees
				task.CustomFields = convertTaskCustomFields(task.CustomFields, cfMapping)

				created, err := destClient.CreateTask(ctx, group.destID, migration.DestWorkspaceID, task)
				if err != nil {
					s.taskMappingRepo.Create(&repository.TaskMapping{
						MigrationID:  migration.Id,
						SourceTaskID: task.Id,
						Status:       repository.TaskMappingStatusFailed,
						ErrorMessage: err.Error(),
					})
					slog.Error("failed to migrate task", "migration_id", migration.Id, "task_name", task.Name, "error", err)
					failCount++
				} else {
					s.taskMappingRepo.Create(&repository.TaskMapping{
						MigrationID:  migration.Id,
						SourceTaskID: task.Id,
						DestTaskID:   created.Id,
						Status:       repository.TaskMappingStatusSuccess,
					})
					slog.Info("task migrated", "migration_id", migration.Id, "dest_task_id", created.Id)
					successCount++
				}
				s.migrationRepo.UpdateProgress(migration.Id, successCount, failCount)
			}
		}
	} else {
		// Non-container source: load all mappings globally
		allMappings, err := s.migrationMappingRepo.GetGlobalByMigrationID(migration.Id)
		if err != nil {
			s.migrationRepo.Complete(migration.Id, repository.MigrationStatusFailed)
			slog.Error("failed to load mappings", "migration_id", migration.Id, "error", err)
			return
		}
		var statusMappings, priorityMappings []MappingItem
		for _, m := range allMappings {
			item := MappingItem{SourceValue: m.SourceValue, DestValue: m.DestValue, Status: string(m.Status)}
			switch m.Type {
			case repository.MappingTypeStatus:
				statusMappings = append(statusMappings, item)
			case repository.MappingTypePriority:
				priorityMappings = append(priorityMappings, item)
			}
		}

		tasks, err := sourceClient.GetTasks(ctx, migration.SourceProjectID)
		if err != nil {
			s.migrationRepo.Complete(migration.Id, repository.MigrationStatusFailed)
			slog.Error("failed to fetch tasks", "migration_id", migration.Id, "error", err)
			return
		}
		s.migrationRepo.UpdateTotalTasks(migration.Id, len(tasks))

		slog.Info("starting migration",
			"migration_id", migration.Id,
			"source", migration.Source,
			"destination", migration.Destination,
			"total_tasks", len(tasks),
		)

		for _, task := range tasks {
			task.Status = mapStatus(task.Status, statusMappings)
			task.Priority = mapPriority(task.Priority, priorityMappings)

			if task.Priority != "" && len(priorityOptions) > 0 {
				fieldGid := priorityOptions["__field_gid__"]
				optionGid := priorityOptions[task.Priority]
				if fieldGid != "" && optionGid != "" {
					task.Priority = fieldGid + ":" + optionGid
				} else {
					task.Priority = ""
				}
			}

			destAssignees := make([]models.TaskAssignee, 0, len(task.Assignees))
			for _, a := range task.Assignees {
				if destID, ok := resolvedAssignees[a.ID]; ok {
					destAssignees = append(destAssignees, models.TaskAssignee{ID: destID})
				}
			}
			task.Assignees = destAssignees
			task.CustomFields = convertTaskCustomFields(task.CustomFields, cfMapping)

			destContainerID := task.DestContainerID
			if destContainerID == "" {
				destContainerID = migration.DestListID
			}
			created, err := destClient.CreateTask(ctx, destContainerID, migration.DestWorkspaceID, task)
			if err != nil {
				s.taskMappingRepo.Create(&repository.TaskMapping{
					MigrationID:  migration.Id,
					SourceTaskID: task.Id,
					Status:       repository.TaskMappingStatusFailed,
					ErrorMessage: err.Error(),
				})
				slog.Error("failed to migrate task", "migration_id", migration.Id, "task_name", task.Name, "error", err)
				failCount++
			} else {
				s.taskMappingRepo.Create(&repository.TaskMapping{
					MigrationID:  migration.Id,
					SourceTaskID: task.Id,
					DestTaskID:   created.Id,
					Status:       repository.TaskMappingStatusSuccess,
				})
				slog.Info("task migrated", "migration_id", migration.Id, "dest_task_id", created.Id)
				successCount++
			}
			s.migrationRepo.UpdateProgress(migration.Id, successCount, failCount)
		}
	}

	finalStatus := repository.MigrationStatusCompleted
	if failCount > 0 {
		finalStatus = repository.MigrationStatusCompletedWithErrors
	}
	s.migrationRepo.Complete(migration.Id, finalStatus)
}

// resolveListIDs returns list IDs from a provider for custom field discovery.
func (s *MigrationService) resolveListIDs(ctx context.Context, provider client.IntegrationProvider, sourceProjectID string) []string {
	if provider == nil {
		return []string{sourceProjectID}
	}
	cp, ok := provider.(client.ContainerProvider)
	if !ok {
		return []string{sourceProjectID}
	}
	containers, err := cp.GetSourceContainers(ctx, sourceProjectID)
	if err != nil || len(containers) == 0 {
		return []string{sourceProjectID}
	}
	ids := make([]string, len(containers))
	for i, c := range containers {
		ids[i] = c.ID
	}
	return ids
}

// ---- Getters ----

func (s *MigrationService) GetMigration(id int64) (repository.Migration, error) {
	migration, err := s.migrationRepo.GetMigration(id)
	if err != nil {
		return repository.Migration{}, fmt.Errorf("get migration: %w", err)
	}
	return migration, nil
}

func (s *MigrationService) GetMigrations() ([]repository.Migration, error) {
	migrations, err := s.migrationRepo.GetMigrations()
	if err != nil {
		return nil, fmt.Errorf("get migrations: %w", err)
	}
	return migrations, nil
}

// ---- Custom field mapping for execution ----

func mapClickUpTypeToAsana(t string) string {
	switch t {
	case "short_text", "text", "url", "email", "phone", "tasks", "location", "users":
		return "text"
	case "number", "currency", "emoji", "automatic_progress", "manual_progress":
		return "number"
	case "drop_down", "checkbox":
		return "enum"
	case "labels":
		return "multi_enum"
	case "date":
		return "date"
	default:
		return "text"
	}
}

type customFieldEntry struct {
	asanaGID    string
	asanaType   string
	clickupType string
	optionMap   map[string]string
}

func (s *MigrationService) buildCustomFieldMapping(
	ctx context.Context,
	fp client.FieldProvider,
	fc client.FieldCreator,
	sourceListIDs []string,
	destWorkspaceId, destProjectId string,
) map[string]customFieldEntry {
	seen := map[string]struct{}{}
	var defs []models.CustomFieldDefinition
	for _, lid := range sourceListIDs {
		d, err := fp.GetFieldDefinitions(ctx, lid)
		if err != nil {
			slog.Warn("could not get field definitions for list, skipping", "listId", lid, "error", err)
			continue
		}
		for _, def := range d {
			if _, dup := seen[def.ID]; !dup {
				seen[def.ID] = struct{}{}
				defs = append(defs, def)
			}
		}
	}

	mapping := make(map[string]customFieldEntry, len(defs))

	for _, def := range defs {
		asanaType := mapClickUpTypeToAsana(def.ClickUpType)

		var optionNames []string
		switch def.ClickUpType {
		case "drop_down":
			sorted := make([]models.CustomFieldOption, len(def.Options))
			copy(sorted, def.Options)
			sort.Slice(sorted, func(i, j int) bool {
				return sorted[i].OrderIndex < sorted[j].OrderIndex
			})
			for _, o := range sorted {
				optionNames = append(optionNames, o.Name)
			}
		case "labels":
			for _, o := range def.Options {
				optionNames = append(optionNames, o.Name)
			}
		case "checkbox":
			optionNames = []string{"True", "False"}
		}

		fieldGID, optionGIDs, found, lookupErr := fc.GetProjectCustomField(ctx, destProjectId, def.Name)
		if lookupErr != nil {
			slog.Warn("could not check existing project fields, will try to create", "field", def.Name, "error", lookupErr)
		}

		if !found {
			var createErr error
			fieldGID, optionGIDs, createErr = fc.CreateCustomField(ctx, destWorkspaceId, def.Name, asanaType, optionNames)
			if createErr != nil {
				if !strings.Contains(createErr.Error(), "already exists with the name") {
					slog.Warn("could not create custom field in destination, skipping", "field", def.Name, "error", createErr)
					continue
				}
				var findErr error
				fieldGID, optionGIDs, findErr = fc.FindCustomFieldByName(ctx, destWorkspaceId, def.Name)
				if findErr != nil {
					slog.Warn("could not locate existing custom field, skipping", "field", def.Name, "error", findErr)
					continue
				}
			}

			if err := fc.AttachCustomFieldToProject(ctx, destProjectId, fieldGID); err != nil {
				slog.Warn("could not attach custom field to project, skipping", "field", def.Name, "error", err)
				continue
			}
		}

		entry := customFieldEntry{
			asanaGID:    fieldGID,
			asanaType:   asanaType,
			clickupType: def.ClickUpType,
			optionMap:   make(map[string]string),
		}

		switch def.ClickUpType {
		case "drop_down":
			sorted := make([]models.CustomFieldOption, len(def.Options))
			copy(sorted, def.Options)
			sort.Slice(sorted, func(i, j int) bool {
				return sorted[i].OrderIndex < sorted[j].OrderIndex
			})
			for i, o := range sorted {
				if i < len(optionGIDs) {
					entry.optionMap[strconv.Itoa(o.OrderIndex)] = optionGIDs[i]
				}
			}
		case "labels":
			for i, o := range def.Options {
				if i < len(optionGIDs) {
					entry.optionMap[o.ID] = optionGIDs[i]
				}
			}
		case "checkbox":
			if len(optionGIDs) >= 2 {
				entry.optionMap["true"] = optionGIDs[0]
				entry.optionMap["false"] = optionGIDs[1]
			}
		}

		mapping[def.ID] = entry
	}

	return mapping
}

func convertTaskCustomFields(
	fields []models.TaskCustomField,
	mapping map[string]customFieldEntry,
) []models.TaskCustomField {
	result := make([]models.TaskCustomField, 0, len(fields))

	for _, cf := range fields {
		entry, ok := mapping[cf.FieldID]
		if !ok {
			continue
		}

		var converted interface{}

		switch entry.clickupType {
		case "short_text", "text", "url", "email", "phone":
			if s, ok := cf.Value.(string); ok {
				converted = s
			}
		case "users", "tasks", "location":
			converted = fmt.Sprintf("%v", cf.Value)
		case "number", "currency", "emoji", "automatic_progress", "manual_progress":
			if n, ok := cf.Value.(float64); ok {
				converted = n
			}
		case "drop_down":
			if n, ok := cf.Value.(float64); ok {
				key := strconv.Itoa(int(n))
				if gid, ok := entry.optionMap[key]; ok {
					converted = gid
				}
			}
		case "labels":
			if arr, ok := cf.Value.([]interface{}); ok {
				gids := make([]string, 0, len(arr))
				for _, item := range arr {
					if m, ok := item.(map[string]interface{}); ok {
						if id, ok := m["id"].(string); ok {
							if gid, ok := entry.optionMap[id]; ok {
								gids = append(gids, gid)
							}
						}
					}
				}
				if len(gids) > 0 {
					converted = gids
				}
			}
		case "checkbox":
			var key string
			switch v := cf.Value.(type) {
			case bool:
				if v {
					key = "true"
				} else {
					key = "false"
				}
			case float64:
				if v != 0 {
					key = "true"
				} else {
					key = "false"
				}
			}
			if key != "" {
				if gid, ok := entry.optionMap[key]; ok {
					converted = gid
				}
			}
		case "date":
			converted = cf.Value
		}

		if converted != nil {
			result = append(result, models.TaskCustomField{
				FieldID: entry.asanaGID,
				Value:   converted,
			})
		}
	}

	return result
}
