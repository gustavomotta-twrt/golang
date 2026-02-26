package service

import (
	"fmt"
	"log/slog"
	"sort"

	"github.com/TWRT/integration-mapper/internal/client"
	"github.com/TWRT/integration-mapper/internal/models"
	"github.com/TWRT/integration-mapper/internal/repository"
)

type MigrationService struct {
	providers            map[string]client.IntegrationProvider
	migrationRepo        *repository.MigrationRepository
	taskMappingRepo      *repository.TaskMappingRepository
	migrationMappingRepo *repository.MigrationMappingRepository
}

func NewMigrationService(
	providers map[string]client.IntegrationProvider,
	migrationRepo *repository.MigrationRepository,
	taskMappingRepo *repository.TaskMappingRepository,
	migrationMappingRepo *repository.MigrationMappingRepository,
) *MigrationService {
	return &MigrationService{
		providers:            providers,
		migrationRepo:        migrationRepo,
		taskMappingRepo:      taskMappingRepo,
		migrationMappingRepo: migrationMappingRepo,
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

type MappingsState struct {
	Status                  []MappingItem
	Priority                []MappingItem
	Assignees               []AssigneeMappingItem
	AvailableDestMembers    []models.Member
	AvailableDestStatuses   []string
	AvailableDestPriorities []string
}

type MappingInput struct {
	Type        repository.MappingType
	SourceValue string
	DestValue   string
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

func (s *MigrationService) discoverAndUpsertMappings(
	migrationID int64,
	sourceClient client.TaskClient,
	sourceProjectId string,
) ([]models.Task, error) {
	tasks, err := sourceClient.GetTasks(sourceProjectId)
	if err != nil {
		return nil, fmt.Errorf("get tasks from source: %w", err)
	}

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
		if err := s.migrationMappingRepo.UpsertPending(
			migrationID, repository.MappingTypeStatus, status, nil,
		); err != nil {
			return nil, fmt.Errorf("upsert status mapping: %w", err)
		}
	}

	for priority := range uniquePriorities {
		if err := s.migrationMappingRepo.UpsertPending(
			migrationID, repository.MappingTypePriority, priority, nil,
		); err != nil {
			return nil, fmt.Errorf("upsert priority mapping: %w", err)
		}
	}

	for _, assignee := range uniqueAssignees {
		metadata := &repository.AssigneeMetadata{
			Name:  assignee.Name,
			Email: assignee.Email,
		}
		if err := s.migrationMappingRepo.UpsertPending(
			migrationID, repository.MappingTypeAssignee, assignee.ID, metadata,
		); err != nil {
			return nil, fmt.Errorf("upsert assignee mapping: %w", err)
		}
	}

	return tasks, nil
}

func (s *MigrationService) buildMappingsState(migration repository.Migration) (*MappingsState, error) {
	allMappings, err := s.migrationMappingRepo.GetByMigrationID(migration.Id, nil)
	if err != nil {
		return nil, fmt.Errorf("get mappings: %w", err)
	}

	destMembers, err := s.getMembersForDestination(migration.Destination, migration.DestWorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("get destination members: %w", err)
	}

	destStatuses, err := s.getAvailableDestStatuses(migration.Destination, migration.DestListID)
	if err != nil {
		return nil, fmt.Errorf("get destination statuses: %w", err)
	}

	destPriorities := s.getAvailableDestPrioritiesForState(migration.Destination, migration.DestListID)

	state := &MappingsState{
		AvailableDestMembers:    destMembers,
		AvailableDestStatuses:   destStatuses,
		AvailableDestPriorities: destPriorities,
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

func (s *MigrationService) CreateMigration(
	source, destination, sourceProjectId, destListId, destWorkspaceId string,
) (int64, *MappingsState, error) {
	migration := &repository.Migration{
		Source:          source,
		Destination:     destination,
		SourceProjectID: sourceProjectId,
		DestListID:      destListId,
		DestWorkspaceID: destWorkspaceId,
		Status:          "pending_configuration",
	}

	migrationID, err := s.migrationRepo.Create(migration)
	if err != nil {
		return 0, nil, fmt.Errorf("create migration: %w", err)
	}

	sourceProvider, err := s.getProvider(source)
	if err != nil {
		return 0, nil, fmt.Errorf("get source provider: %w", err)
	}
	if _, err := s.discoverAndUpsertMappings(migrationID, sourceProvider, sourceProjectId); err != nil {
		return 0, nil, fmt.Errorf("discover mappings: %w", err)
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
	if _, err := s.discoverAndUpsertMappings(migrationID, sourceProvider, migration.SourceProjectID); err != nil {
		return nil, fmt.Errorf("sync mappings: %w", err)
	}

	return s.buildMappingsState(migration)
}

func (s *MigrationService) SaveMappings(migrationID int64, mappings []MappingInput) (*MappingsState, error) {
	migration, err := s.migrationRepo.GetMigration(migrationID)
	if err != nil {
		return nil, fmt.Errorf("get migration: %w", err)
	}

	for _, m := range mappings {
		if err := s.migrationMappingRepo.UpdateMapping(
			migrationID, m.Type, m.SourceValue, m.DestValue,
		); err != nil {
			return nil, fmt.Errorf("save mapping %s/%s: %w", m.Type, m.SourceValue, err)
		}
	}

	allMapped, err := s.migrationMappingRepo.AllMapped(migrationID)
	if err != nil {
		return nil, fmt.Errorf("check all mapped: %w", err)
	}
	if allMapped {
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
		return fmt.Errorf("there are pending mappings — configure all before starting")
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
	tasks, err := sourceClient.GetTasks(migration.SourceProjectID)
	if err != nil {
		s.migrationRepo.Complete(migration.Id, "failed")
		slog.Error("failed to fetch tasks", "migration_id", migration.Id, "error", err)
		return
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

	slog.Info("starting migration",
		"migration_id", migration.Id,
		"source", migration.Source,
		"destination", migration.Destination,
		"total_tasks", len(tasks),
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

		created, err := destClient.CreateTask(migration.DestListID, migration.DestWorkspaceID, task)
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
