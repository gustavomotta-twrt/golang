package service

import (
	"fmt"

	"github.com/TWRT/integration-mapper/internal/client"
	"github.com/TWRT/integration-mapper/internal/models"
	"github.com/TWRT/integration-mapper/internal/repository"
)

type MigrationService struct {
	asanaClient           client.TaskClient
	clickupClient         client.TaskClient
	asanaMemberProvider   client.MemberProvider
	clickupMemberProvider client.MemberProvider
	migrationRepo         *repository.MigrationRepository
	taskMappingRepo       *repository.TaskMappingRepository
	migrationMappingRepo  *repository.MigrationMappingRepository
}

func NewMigrationService(
	asanaClient client.TaskClient,
	clickupClient client.TaskClient,
	asanaMemberProvider client.MemberProvider,
	clickupMemberProvider client.MemberProvider,
	migrationRepo *repository.MigrationRepository,
	taskMappingRepo *repository.TaskMappingRepository,
	migrationMappingRepo *repository.MigrationMappingRepository,
) *MigrationService {
	return &MigrationService{
		asanaClient:           asanaClient,
		clickupClient:         clickupClient,
		asanaMemberProvider:   asanaMemberProvider,
		clickupMemberProvider: clickupMemberProvider,
		migrationRepo:         migrationRepo,
		taskMappingRepo:       taskMappingRepo,
		migrationMappingRepo:  migrationMappingRepo,
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
	Status               []MappingItem
	Priority             []MappingItem
	Assignees            []AssigneeMappingItem
	AvailableDestMembers []models.Member
}

type MappingInput struct {
	Type        repository.MappingType
	SourceValue string
	DestValue   string
}

func (s *MigrationService) getClients(source, destination string) (sourceClient, destClient client.TaskClient) {
	if source == "clickup" {
		sourceClient = s.clickupClient
	} else {
		sourceClient = s.asanaClient
	}
	if destination == "clickup" {
		destClient = s.clickupClient
	} else {
		destClient = s.asanaClient
	}
	return
}

func (s *MigrationService) getMembersForDestination(destination, destWorkspaceId string) ([]models.Member, error) {
	if destWorkspaceId == "" {
		return nil, fmt.Errorf("dest_workspace_id is required for destination %s", destination)
	}
	if destination == "clickup" {
		return s.clickupMemberProvider.GetMembers(destWorkspaceId)
	}
	return s.asanaMemberProvider.GetMembers(destWorkspaceId)
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

	state := &MappingsState{
		AvailableDestMembers: destMembers,
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

	sourceClient, _ := s.getClients(source, destination)

	if _, err := s.discoverAndUpsertMappings(migrationID, sourceClient, sourceProjectId); err != nil {
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

	sourceClient, _ := s.getClients(migration.Source, migration.Destination)

	if _, err := s.discoverAndUpsertMappings(migrationID, sourceClient, migration.SourceProjectID); err != nil {
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

	sourceClient, destClient := s.getClients(migration.Source, migration.Destination)

	if err := s.migrationRepo.UpdateStatus(migrationID, "running"); err != nil {
		return fmt.Errorf("update migration status: %w", err)
	}

	go s.executeMigration(sourceClient, destClient, migration)

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
		fmt.Printf("Erro ao buscar tasks: %v\n", err)
		return
	}

	s.migrationRepo.UpdateTotalTasks(migration.Id, len(tasks))

	allMappings, err := s.migrationMappingRepo.GetByMigrationID(migration.Id, nil)
	if err != nil {
		s.migrationRepo.Complete(migration.Id, "failed")
		fmt.Printf("Erro ao carregar mapeamentos: %v\n", err)
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

	fmt.Printf("🚀 Iniciando migração: %s → %s\n", migration.Source, migration.Destination)
	fmt.Printf("📋 Total de tasks: %d\n", len(tasks))

	successCount := 0
	failCount := 0

	for _, task := range tasks {
		fmt.Printf("⏳ Migrando: [%s] %s...\n", task.Id, task.Name)

		task.Status = mapStatus(task.Status, statusMappings)
		task.Priority = mapPriority(task.Priority, priorityMappings)

		destAssignees := make([]models.TaskAssignee, 0, len(task.Assignees))
		for _, a := range task.Assignees {
			if destID, ok := resolvedAssignees[a.ID]; ok {
				destAssignees = append(destAssignees, models.TaskAssignee{ID: destID})
			}
		}
		task.Assignees = destAssignees

		created, err := destClient.CreateTask(migration.DestListID, task)
		if err != nil {
			s.taskMappingRepo.Create(&repository.TaskMapping{
				MigrationID:  migration.Id,
				SourceTaskID: task.Id,
				Status:       "failed",
				ErrorMessage: err.Error(),
			})
			fmt.Printf("❌ Erro ao migrar task %s: %v\n", task.Name, err)
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
		fmt.Printf("✅ Migrada! ID destino: %s\n", created.Id)
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
