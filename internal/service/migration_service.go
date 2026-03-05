package service

import (
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/TWRT/integration-mapper/internal/client"
	"github.com/TWRT/integration-mapper/internal/models"
	"github.com/TWRT/integration-mapper/internal/repository"
)

type MigrationService struct {
	providers              map[string]client.IntegrationProvider
	migrationRepo          *repository.MigrationRepository
	taskMappingRepo        *repository.TaskMappingRepository
	migrationMappingRepo   *repository.MigrationMappingRepository
	containerMappingRepo   *repository.ContainerMappingRepository
}

func NewMigrationService(
	providers map[string]client.IntegrationProvider,
	migrationRepo *repository.MigrationRepository,
	taskMappingRepo *repository.TaskMappingRepository,
	migrationMappingRepo *repository.MigrationMappingRepository,
	containerMappingRepo *repository.ContainerMappingRepository,
) *MigrationService {
	return &MigrationService{
		providers:            providers,
		migrationRepo:        migrationRepo,
		taskMappingRepo:      taskMappingRepo,
		migrationMappingRepo: migrationMappingRepo,
		containerMappingRepo: containerMappingRepo,
	}
}

type MappingItem struct {
	SourceValue string
	DestValue   *string
	Status      string
}

type AssigneeMappingItem struct {
	MappingItem
	Name  string
	Email string
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

type ContainerMappingItem struct {
	SourceID   string
	SourceName string
	DestID     *string
	DestName   *string
	Status     string
}

type AvailableContainer struct {
	ID   string
	Name string
}

type MappingsState struct {
	Status                  []MappingItem
	Priority                []MappingItem
	Assignees               []AssigneeMappingItem
	AvailableDestMembers    []models.Member
	AvailableDestStatuses   []string
	AvailableDestPriorities []string
	DiscoveredCustomFields  []CustomFieldState
	ContainerMappings       []ContainerMappingItem
	AvailableDestContainers []AvailableContainer
}

type MappingInput struct {
	Type        repository.MappingType
	SourceValue string
	DestValue   string
}

type ContainerMappingInput struct {
	SourceID   string
	DestID     string
	DestName   string
}

func (s *MigrationService) getProvider(name string) (client.IntegrationProvider, error) {
	p, ok := s.providers[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
	return p, nil
}

func (s *MigrationService) getMembersForDestination(destination, destWorkspaceId string) ([]models.Member, error) {
	if destWorkspaceId == "" {
		return nil, fmt.Errorf("dest_workspace_id is required for destination %s", destination)
	}
	p, err := s.getProvider(destination)
	if err != nil {
		return nil, err
	}
	return p.GetMembers(destWorkspaceId)
}

func (s *MigrationService) getAvailableDestStatuses(destination, destListId string) ([]string, error) {
	p, err := s.getProvider(destination)
	if err != nil {
		return nil, err
	}
	return p.GetListStatuses(destListId)
}

func (s *MigrationService) getAvailableDestPrioritiesForState(destination, destListID string) []string {
	provider, err := s.getProvider(destination)
	if err != nil {
		return getAvailableDestPriorities(destination)
	}
	lookup, ok := provider.(client.PriorityLookup)
	if !ok {
		return getAvailableDestPriorities(destination)
	}
	options, err := lookup.GetProjectCustomFieldOptions(destListID)
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

func (s *MigrationService) discoverCustomFields(
	sourceClient client.IntegrationProvider,
	sourceProjectID string,
) []models.CustomFieldDefinition {
	fp, ok := sourceClient.(client.FieldProvider)
	if !ok {
		return []models.CustomFieldDefinition{}
	}

	listIDs := s.resolveListIDs(sourceClient, sourceProjectID)

	seen := map[string]struct{}{}
	var allDefs []models.CustomFieldDefinition
	for _, lid := range listIDs {
		defs, err := fp.GetFieldDefinitions(lid)
		if err != nil {
			slog.Warn("could not discover custom fields for list, skipping", "listId", lid, "error", err)
			continue
		}
		for _, d := range defs {
			if _, dup := seen[d.ID]; !dup {
				seen[d.ID] = struct{}{}
				allDefs = append(allDefs, d)
			}
		}
	}
	return allDefs
}

// resolveListIDs returns the individual list IDs to query for custom fields.
// If the provider supports containers (ClickUp space → lists), it fetches
// the lists from the space. Otherwise it returns the ID as-is.
func (s *MigrationService) resolveListIDs(provider client.IntegrationProvider, sourceProjectID string) []string {
	if provider == nil {
		return []string{sourceProjectID}
	}
	cp, ok := provider.(client.ContainerProvider)
	if !ok {
		return []string{sourceProjectID}
	}
	containers, err := cp.GetSourceContainers(sourceProjectID)
	if err != nil || len(containers) == 0 {
		return []string{sourceProjectID}
	}
	ids := make([]string, len(containers))
	for i, c := range containers {
		ids[i] = c.ID
	}
	return ids
}

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

func (s *MigrationService) upsertTaskMappings(migrationID int64, tasks []models.Task) error {
	uniqueStatuses := make(map[string]struct{})
	uniquePriorities := make(map[string]struct{})
	uniqueAssignees := make(map[string]models.TaskAssignee)

	for _, task := range tasks {
		if task.Status != "" {
			uniqueStatuses[task.Status] = struct{}{}
		}
		if task.Priority != "" {
			uniquePriorities[task.Priority] = struct{}{}
		}
		for _, a := range task.Assignees {
			if a.ID != "" {
				uniqueAssignees[a.ID] = a
			}
		}
	}

	for status := range uniqueStatuses {
		if err := s.migrationMappingRepo.UpsertPending(migrationID, repository.MappingTypeStatus, status, nil); err != nil {
			return fmt.Errorf("upsert status mapping: %w", err)
		}
	}
	for priority := range uniquePriorities {
		if err := s.migrationMappingRepo.UpsertPending(migrationID, repository.MappingTypePriority, priority, nil); err != nil {
			return fmt.Errorf("upsert priority mapping: %w", err)
		}
	}
	for _, assignee := range uniqueAssignees {
		metadata := &repository.AssigneeMetadata{Name: assignee.Name, Email: assignee.Email}
		if err := s.migrationMappingRepo.UpsertPending(migrationID, repository.MappingTypeAssignee, assignee.ID, metadata); err != nil {
			return fmt.Errorf("upsert assignee mapping: %w", err)
		}
	}
	return nil
}

// discoverAndUpsertMappingsFromContainers fetches tasks from all containers and discovers unique
// status/priority/assignee values for mapping. For ClickUp source it also discovers custom fields.
func (s *MigrationService) discoverAndUpsertMappingsFromContainers(
	migrationID int64,
	sourceProvider client.IntegrationProvider,
	sourceID string,
	sourcePlatform string,
) ([]models.Task, error) {
	cp, hasContainers := sourceProvider.(client.ContainerProvider)

	var allTasks []models.Task

	if hasContainers {
		containers, err := cp.GetSourceContainers(sourceID)
		if err != nil {
			return nil, fmt.Errorf("get source containers for mapping discovery: %w", err)
		}
		for _, c := range containers {
			tasks, err := cp.GetTasksByContainer(c.ID)
			if err != nil {
				slog.Warn("could not get tasks for container, skipping", "container", c.Name, "error", err)
				continue
			}
			allTasks = append(allTasks, tasks...)
		}
	} else {
		tasks, err := sourceProvider.GetTasks(sourceID)
		if err != nil {
			return nil, fmt.Errorf("get tasks from source: %w", err)
		}
		allTasks = tasks
	}

	if err := s.upsertTaskMappings(migrationID, allTasks); err != nil {
		return nil, err
	}

	return allTasks, nil
}

func (s *MigrationService) loadCustomFieldsState(migrationID int64) []CustomFieldState {
	dbRows, err := s.migrationMappingRepo.GetCustomFields(migrationID)
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

func (s *MigrationService) getDestContainerID(migration repository.Migration) string {
	if migration.DestSpaceID != "" {
		return migration.DestSpaceID
	}
	return migration.DestListID
}

func (s *MigrationService) buildContainerMappingsState(migration repository.Migration) ([]ContainerMappingItem, []AvailableContainer, error) {
	containerMappings, err := s.containerMappingRepo.GetByMigrationID(migration.Id)
	if err != nil {
		return nil, nil, fmt.Errorf("get container mappings: %w", err)
	}

	items := make([]ContainerMappingItem, len(containerMappings))
	for i, cm := range containerMappings {
		items[i] = ContainerMappingItem{
			SourceID:   cm.SourceID,
			SourceName: cm.SourceName,
			DestID:     cm.DestID,
			DestName:   cm.DestName,
			Status:     cm.Status,
		}
	}

	destProvider, err := s.getProvider(migration.Destination)
	if err != nil {
		return items, nil, nil
	}

	cp, ok := destProvider.(client.ContainerProvider)
	if !ok {
		return items, nil, nil
	}

	destContainerID := s.getDestContainerID(migration)
	destContainers, err := cp.GetDestContainers(destContainerID)
	if err != nil {
		slog.Warn("could not fetch dest containers for state", "error", err)
		return items, nil, nil
	}

	available := make([]AvailableContainer, len(destContainers))
	for i, dc := range destContainers {
		available[i] = AvailableContainer{ID: dc.ID, Name: dc.Name}
	}

	return items, available, nil
}

func (s *MigrationService) buildMappingsState(
	migration repository.Migration,
) (*MappingsState, error) {
	allMappings, err := s.migrationMappingRepo.GetByMigrationID(migration.Id, nil)
	if err != nil {
		return nil, fmt.Errorf("get mappings: %w", err)
	}

	destMembers, err := s.getMembersForDestination(migration.Destination, migration.DestWorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("get destination members: %w", err)
	}

	// For ClickUp destination, use the first mapped container list ID for status discovery.
	// If no container is mapped yet, statuses are deferred (empty list — OK).
	// For Asana destination, DestListID is the project GID and statuses are fixed.
	var destStatuses []string
	var destPriorities []string

	if migration.Destination == "clickup" {
		statusListID := ""
		if cms, err := s.containerMappingRepo.GetByMigrationID(migration.Id); err == nil {
			for _, cm := range cms {
				if cm.DestID != nil {
					statusListID = *cm.DestID
					break
				}
			}
		}
		if statusListID != "" {
			if ss, err := s.getAvailableDestStatuses(migration.Destination, statusListID); err == nil {
				destStatuses = ss
			} else {
				slog.Warn("could not fetch dest statuses, will return empty", "error", err)
			}
			destPriorities = s.getAvailableDestPrioritiesForState(migration.Destination, statusListID)
		} else {
			destPriorities = getAvailableDestPriorities(migration.Destination)
		}
	} else {
		if ss, err := s.getAvailableDestStatuses(migration.Destination, migration.DestListID); err != nil {
			return nil, fmt.Errorf("get destination statuses: %w", err)
		} else {
			destStatuses = ss
		}
		destPriorities = s.getAvailableDestPrioritiesForState(migration.Destination, migration.DestListID)
	}

	containerItems, availableContainers, err := s.buildContainerMappingsState(migration)
	if err != nil {
		return nil, fmt.Errorf("build container mappings state: %w", err)
	}

	state := &MappingsState{
		AvailableDestMembers:    destMembers,
		AvailableDestStatuses:   destStatuses,
		AvailableDestPriorities: destPriorities,
		DiscoveredCustomFields:  s.loadCustomFieldsState(migration.Id),
		ContainerMappings:       containerItems,
		AvailableDestContainers: availableContainers,
	}

	for _, m := range allMappings {
		item := MappingItem{
			SourceValue: m.SourceValue,
			DestValue:   m.DestValue,
			Status:      string(m.Status),
		}
		switch m.Type {
		case repository.MappingTypeStatus:
			state.Status = append(state.Status, item)
		case repository.MappingTypePriority:
			state.Priority = append(state.Priority, item)
		case repository.MappingTypeAssignee:
			assigneeItem := AssigneeMappingItem{MappingItem: item}
			if m.Metadata != nil {
				assigneeItem.Name = m.Metadata.Name
				assigneeItem.Email = m.Metadata.Email
			}
			state.Assignees = append(state.Assignees, assigneeItem)
		}
	}

	return state, nil
}

// CreateMigrationInput holds parameters for creating a migration.
// DestSpaceID is the ClickUp space GID (when dest=clickup) or empty (when dest=asana, use DestListID as project GID).
type CreateMigrationInput struct {
	Source          string
	Destination     string
	SourceProjectID string // Asana project GID or ClickUp space GID
	DestListID      string // Asana project GID when dest=asana (used for project ID)
	DestWorkspaceID string
	DestSpaceID     string // ClickUp space GID when dest=clickup
}

func (s *MigrationService) CreateMigration(input CreateMigrationInput) (int64, *MappingsState, error) {
	migration := &repository.Migration{
		Source:          input.Source,
		Destination:     input.Destination,
		SourceProjectID: input.SourceProjectID,
		DestListID:      input.DestListID,
		DestWorkspaceID: input.DestWorkspaceID,
		DestSpaceID:     input.DestSpaceID,
		Status:          "pending_configuration",
	}

	migrationID, err := s.migrationRepo.Create(migration)
	if err != nil {
		return 0, nil, fmt.Errorf("create migration: %w", err)
	}

	sourceProvider, err := s.getProvider(input.Source)
	if err != nil {
		return 0, nil, fmt.Errorf("get source provider: %w", err)
	}

	// Discover source containers (Asana sections or ClickUp lists)
	if cp, ok := sourceProvider.(client.ContainerProvider); ok {
		containers, err := cp.GetSourceContainers(input.SourceProjectID)
		if err != nil {
			return 0, nil, fmt.Errorf("discover source containers: %w", err)
		}
		for _, c := range containers {
			if err := s.containerMappingRepo.Upsert(migrationID, c.ID, c.Name); err != nil {
				return 0, nil, fmt.Errorf("upsert container mapping: %w", err)
			}
		}
	}

	// Discover status/priority/assignee mappings using all tasks across containers
	if _, err := s.discoverAndUpsertMappingsFromContainers(migrationID, sourceProvider, input.SourceProjectID, input.Source); err != nil {
		return 0, nil, fmt.Errorf("discover mappings: %w", err)
	}

	for _, f := range s.discoverCustomFields(sourceProvider, input.SourceProjectID) {
		if err := s.migrationMappingRepo.UpsertCustomField(migrationID, f.ID, f.Name, f.ClickUpType); err != nil {
			slog.Warn("could not persist custom field", "field", f.Name, "error", err)
		}
	}

	migration.Id = migrationID
	state, err := s.buildMappingsState(*migration)
	if err != nil {
		return 0, nil, fmt.Errorf("build mappings state: %w", err)
	}

	return migrationID, state, nil
}

func (s *MigrationService) SyncMappings(migrationID int64) (*MappingsState, error) {
	migration, err := s.migrationRepo.GetMigration(migrationID)
	if err != nil {
		return nil, fmt.Errorf("get migration: %w", err)
	}

	sourceProvider, err := s.getProvider(migration.Source)
	if err != nil {
		return nil, fmt.Errorf("get source provider: %w", err)
	}

	if cp, ok := sourceProvider.(client.ContainerProvider); ok {
		containers, err := cp.GetSourceContainers(migration.SourceProjectID)
		if err != nil {
			return nil, fmt.Errorf("sync containers: %w", err)
		}
		for _, c := range containers {
			if err := s.containerMappingRepo.Upsert(migrationID, c.ID, c.Name); err != nil {
				slog.Warn("could not upsert container on sync", "container", c.Name, "error", err)
			}
		}
	}

	if _, err := s.discoverAndUpsertMappingsFromContainers(migrationID, sourceProvider, migration.SourceProjectID, migration.Source); err != nil {
		return nil, fmt.Errorf("sync mappings: %w", err)
	}

	for _, f := range s.discoverCustomFields(sourceProvider, migration.SourceProjectID) {
		if err := s.migrationMappingRepo.UpsertCustomField(migrationID, f.ID, f.Name, f.ClickUpType); err != nil {
			slog.Warn("could not persist custom field on sync", "field", f.Name, "error", err)
		}
	}

	return s.buildMappingsState(migration)
}

func (s *MigrationService) SaveMappings(
	migrationID int64,
	mappings []MappingInput,
	containerMappings []ContainerMappingInput,
	customFieldSelections []CustomFieldSelection,
) (*MappingsState, error) {
	migration, err := s.migrationRepo.GetMigration(migrationID)
	if err != nil {
		return nil, fmt.Errorf("get migration: %w", err)
	}

	for _, cm := range containerMappings {
		if err := s.containerMappingRepo.UpdateMapping(migrationID, cm.SourceID, cm.DestID, cm.DestName); err != nil {
			return nil, fmt.Errorf("save container mapping %s: %w", cm.SourceID, err)
		}
	}

	for _, m := range mappings {
		if err := s.migrationMappingRepo.UpdateMapping(migrationID, m.Type, m.SourceValue, m.DestValue); err != nil {
			return nil, fmt.Errorf("save mapping %s/%s: %w", m.Type, m.SourceValue, err)
		}
	}

	for _, sel := range customFieldSelections {
		if err := s.migrationMappingRepo.UpdateCustomFieldEnabled(migrationID, sel.FieldID, sel.Enabled); err != nil {
			return nil, fmt.Errorf("save custom field selection %s: %w", sel.FieldID, err)
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
		if err := s.migrationRepo.UpdateStatus(migrationID, "ready_to_start"); err != nil {
			return nil, fmt.Errorf("update migration status: %w", err)
		}
	} else {
		if err := s.migrationRepo.UpdateStatus(migrationID, "pending_configuration"); err != nil {
			return nil, fmt.Errorf("update migration status: %w", err)
		}
	}

	return s.buildMappingsState(migration)
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
		return fmt.Errorf("get destination provider: %w", err)
	}
	if err := s.migrationRepo.UpdateStatus(migrationID, "running"); err != nil {
		return fmt.Errorf("update migration status: %w", err)
	}

	go s.executeMigration(sourceProvider, destProvider, migration)

	return nil
}

func (s *MigrationService) executeMigration(
	sourceClient client.TaskClient,
	destClient client.TaskClient,
	migration repository.Migration,
) {
	// Load container mappings to iterate per section/list
	containerMappings, err := s.containerMappingRepo.GetByMigrationID(migration.Id)
	if err != nil {
		s.migrationRepo.Complete(migration.Id, "failed")
		slog.Error("failed to load container mappings", "migration_id", migration.Id, "error", err)
		return
	}

	cp, hasContainerProvider := sourceClient.(client.ContainerProvider)

	// Fetch all tasks across containers
	var tasks []models.Task
	if hasContainerProvider && len(containerMappings) > 0 {
		for _, cm := range containerMappings {
			if cm.DestID == nil {
				slog.Warn("container has no dest mapping, skipping", "source_container", cm.SourceName)
				continue
			}
			containerTasks, err := cp.GetTasksByContainer(cm.SourceID)
			if err != nil {
				s.migrationRepo.Complete(migration.Id, "failed")
				slog.Error("failed to fetch tasks for container", "container", cm.SourceName, "error", err)
				return
			}
			// Tag each task with its dest container ID for routing
			for i := range containerTasks {
				containerTasks[i].DestContainerID = *cm.DestID
				// For Asana dest with section, encode as "projectId|sectionId"
				if migration.Destination == "asana" {
					containerTasks[i].DestContainerID = migration.DestListID + "|" + *cm.DestID
				}
			}
			tasks = append(tasks, containerTasks...)
		}
	} else {
		tasks, err = sourceClient.GetTasks(migration.SourceProjectID)
		if err != nil {
			s.migrationRepo.Complete(migration.Id, "failed")
			slog.Error("failed to fetch tasks", "migration_id", migration.Id, "error", err)
			return
		}
	}

	s.migrationRepo.UpdateTotalTasks(migration.Id, len(tasks))

	allMappings, err := s.migrationMappingRepo.GetByMigrationID(migration.Id, nil)
	if err != nil {
		s.migrationRepo.Complete(migration.Id, "failed")
		slog.Error("failed to load mappings", "migration_id", migration.Id, "error", err)
		return
	}

	var statusMappings, priorityMappings []MappingItem
	resolvedAssignees := make(map[string]string)

	for _, m := range allMappings {
		item := MappingItem{
			SourceValue: m.SourceValue,
			DestValue:   m.DestValue,
			Status:      string(m.Status),
		}
		switch m.Type {
		case repository.MappingTypeStatus:
			statusMappings = append(statusMappings, item)
		case repository.MappingTypePriority:
			priorityMappings = append(priorityMappings, item)
		case repository.MappingTypeAssignee:
			if m.DestValue != nil {
				resolvedAssignees[m.SourceValue] = *m.DestValue
			}
		}
	}

	cfMapping := map[string]customFieldEntry{}
	if fp, ok := sourceClient.(client.FieldProvider); ok {
		if fc, ok := destClient.(client.FieldCreator); ok {
			sourceProvider, _ := sourceClient.(client.IntegrationProvider)
			listIDs := s.resolveListIDs(sourceProvider, migration.SourceProjectID)
			cfMapping = s.buildCustomFieldMapping(
				fp, fc,
				listIDs,
				migration.DestWorkspaceID,
				migration.DestListID,
			)
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

	slog.Info("starting migration",
		"migration_id", migration.Id,
		"source", migration.Source,
		"destination", migration.Destination,
		"total_tasks", len(tasks),
		"custom_fields_mapped", len(cfMapping),
	)

	successCount := 0
	failCount := 0

	priorityOptions := map[string]string{}
	if lookup, ok := destClient.(client.PriorityLookup); ok {
		options, err := lookup.GetProjectCustomFieldOptions(migration.DestListID)
		if err != nil {
			s.migrationRepo.Complete(migration.Id, "failed")
			slog.Error("failed to fetch priority options", "migration_id", migration.Id, "error", err)
			return
		}
		priorityOptions = options
	}

	for _, task := range tasks {
		slog.Info("migrating task", "migration_id", migration.Id, "task_id", task.Id, "task_name", task.Name)

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
		created, err := destClient.CreateTask(destContainerID, migration.DestWorkspaceID, task)
		if err != nil {
			s.taskMappingRepo.Create(&repository.TaskMapping{
				MigrationID:  migration.Id,
				SourceTaskID: task.Id,
				Status:       "failed",
				ErrorMessage: err.Error(),
			})
			slog.Error("failed to migrate task", "migration_id", migration.Id, "task_name", task.Name, "error", err)
			failCount++
			s.migrationRepo.UpdateProgress(migration.Id, successCount, failCount)
			continue
		}

		s.taskMappingRepo.Create(&repository.TaskMapping{
			MigrationID:  migration.Id,
			SourceTaskID: task.Id,
			DestTaskID:   created.Id,
			Status:       "success",
		})
		slog.Info("task migrated", "migration_id", migration.Id, "dest_task_id", created.Id)
		successCount++
		s.migrationRepo.UpdateProgress(migration.Id, successCount, failCount)
	}

	finalStatus := "completed"
	if failCount > 0 {
		finalStatus = "completed_with_errors"
	}
	s.migrationRepo.Complete(migration.Id, finalStatus)
}

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
	fp client.FieldProvider,
	fc client.FieldCreator,
	sourceListIDs []string,
	destWorkspaceId, destProjectId string,
) map[string]customFieldEntry {
	seen := map[string]struct{}{}
	var defs []models.CustomFieldDefinition
	for _, lid := range sourceListIDs {
		d, err := fp.GetFieldDefinitions(lid)
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

		fieldGID, optionGIDs, found, lookupErr := fc.GetProjectCustomField(destProjectId, def.Name)
		if lookupErr != nil {
			slog.Warn("could not check existing project fields, will try to create", "field", def.Name, "error", lookupErr)
		}

		if !found {
			var createErr error
			fieldGID, optionGIDs, createErr = fc.CreateCustomField(destWorkspaceId, def.Name, asanaType, optionNames)
			if createErr != nil {
				if !strings.Contains(createErr.Error(), "already exists with the name") {
					slog.Warn("could not create custom field in destination, skipping", "field", def.Name, "error", createErr)
					continue
				}
				var findErr error
				fieldGID, optionGIDs, findErr = fc.FindCustomFieldByName(destWorkspaceId, def.Name)
				if findErr != nil {
					slog.Warn("could not locate existing custom field, skipping", "field", def.Name, "error", findErr)
					continue
				}
			}

			if err := fc.AttachCustomFieldToProject(destProjectId, fieldGID); err != nil {
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
			case string:
				key = strings.ToLower(v)
			}
			if key != "" {
				if gid, ok := entry.optionMap[key]; ok {
					converted = gid
				}
			}

		case "date":
			if ms, ok := cf.Value.(string); ok {
				msInt, err := strconv.ParseInt(ms, 10, 64)
				if err == nil {
					t := time.UnixMilli(msInt).UTC()
					converted = map[string]interface{}{"date": t.Format("2006-01-02")}
				}
			}
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
