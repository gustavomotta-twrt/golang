package service

import (
	"encoding/json"
	"fmt"

	"github.com/TWRT/integration-mapper/internal/client"
	"github.com/TWRT/integration-mapper/internal/models"
	"github.com/TWRT/integration-mapper/internal/repository"
)

type MigrationService struct {
	asanaClient              client.TaskClient
	clickupClient            client.TaskClient
	asanaMemberProvider      client.MemberProvider
	clickupMemberProvider    client.MemberProvider
	migrationRepo            *repository.MigrationRepository
	taskMappingRepo          *repository.TaskMappingRepository
	pendingAssigneeMappingRepo *repository.PendingAssigneeMappingRepository
}

func NewMigrationService(
	asanaClient client.TaskClient,
	clickupClient client.TaskClient,
	asanaMemberProvider client.MemberProvider,
	clickupMemberProvider client.MemberProvider,
	migrationRepo *repository.MigrationRepository,
	taskMappingRepo *repository.TaskMappingRepository,
	pendingAssigneeMappingRepo *repository.PendingAssigneeMappingRepository,
) *MigrationService {
	return &MigrationService{
		asanaClient:              asanaClient,
		clickupClient:            clickupClient,
		asanaMemberProvider:      asanaMemberProvider,
		clickupMemberProvider:    clickupMemberProvider,
		migrationRepo:            migrationRepo,
		taskMappingRepo:          taskMappingRepo,
		pendingAssigneeMappingRepo: pendingAssigneeMappingRepo,
	}
}

func mapStatus(taskStatus string, statusMappings []models.StatusMapping) string {
	for _, mapping := range statusMappings {
		if mapping.SourceStatus == taskStatus {
			return mapping.DestStatus
		}
	}
	return "to do"
}

func (s *MigrationService) getDestMemberProvider(destination string) client.MemberProvider {
	if destination == "clickup" {
		return s.clickupMemberProvider
	}
	return s.asanaMemberProvider
}

func (s *MigrationService) getMembersForDestination(destination, destListId, destWorkspaceId string) ([]models.Member, error) {
	provider := s.getDestMemberProvider(destination)
	if destination == "clickup" {
		if destWorkspaceId == "" {
			return nil, fmt.Errorf("dest_workspace_id is required when destination is clickup")
		}
		return provider.GetMembers(destWorkspaceId)
	}
	if destination == "asana" {
		if destWorkspaceId == "" {
			return nil, fmt.Errorf("dest_workspace_id is required when destination is asana")
		}
		return provider.GetMembers(destWorkspaceId)
	}
	return nil, fmt.Errorf("unsupported destination: %s", destination)
}

func (s *MigrationService) executeMigration(
	sourceClient client.TaskClient,
	source string,
	destClient client.TaskClient,
	destination string,
	migrationID int64,
	sourceProjectId string,
	destListId string,
	destWorkspaceId string,
	statusMappings []models.StatusMapping,
	assigneeMappings []models.AssigneeMapping,
) {
	tasks, err := sourceClient.GetTasks(sourceProjectId)
	if err != nil {
		s.migrationRepo.Complete(migrationID, "failed")
		fmt.Printf("❌ Erro ao buscar tasks: %v\n", err)
		return
	}

	fmt.Printf("🚀 Iniciando migração: %s → %s\n", source, destination)
	fmt.Printf("📋 Total de tasks encontradas: %d\n", len(tasks))

	s.migrationRepo.UpdateTotalTasks(migrationID, len(tasks))
	s.migrationRepo.UpdateStatus(migrationID, "running")

	destMembers, err := s.getMembersForDestination(destination, destListId, destWorkspaceId)
	if err != nil {
		s.migrationRepo.Complete(migrationID, "failed")
		fmt.Printf("❌ Erro ao buscar membros do destination: %v\n", err)
		return
	}

	emailToDestID := make(map[string]string)
	for _, m := range destMembers {
		if m.Email != "" {
			emailToDestID[m.Email] = m.ID
		}
	}

	uniqueAssignees := make(map[string]models.TaskAssignee)
	for _, task := range tasks {
		for _, a := range task.Assignees {
			if a.ID != "" {
				uniqueAssignees[a.ID] = a
			}
		}
	}

	resolvedAssignees := make(map[string]string)
	for _, m := range assigneeMappings {
		resolvedAssignees[m.SourceUserId] = m.DestUserId
	}

	hasPending := false
	for sourceID, assignee := range uniqueAssignees {
		if _, ok := resolvedAssignees[sourceID]; ok {
			continue
		}
		destID, ok := emailToDestID[assignee.Email]
		if ok && assignee.Email != "" {
			resolvedAssignees[sourceID] = destID
			continue
		}
		hasPending = true
		if err := s.pendingAssigneeMappingRepo.Create(&repository.PendingAssigneeMapping{
			MigrationID:     migrationID,
			SourceUserId:    assignee.ID,
			SourceUserName:  assignee.Name,
			SourceUserEmail: assignee.Email,
		}); err != nil {
			s.migrationRepo.Complete(migrationID, "failed")
			fmt.Printf("❌ Erro ao salvar assignee pendente: %v\n", err)
			return
		}
	}

	if hasPending {
		s.migrationRepo.UpdateStatus(migrationID, "pending_assignee_mapping")
		fmt.Printf("⏸️ Migração pausada: assignees pendentes de mapeamento\n")
		return
	}

	s.continueExecution(sourceClient, destClient, source, destination, migrationID, destListId, tasks, statusMappings, resolvedAssignees)
}

func (s *MigrationService) continueExecution(
	sourceClient client.TaskClient,
	destClient client.TaskClient,
	source string,
	destination string,
	migrationID int64,
	destListId string,
	tasks []models.Task,
	statusMappings []models.StatusMapping,
	resolvedAssignees map[string]string,
) {
	successCount := 0
	failCount := 0

	for _, task := range tasks {
		fmt.Printf("⏳ Migrando: [%s] %s...\n", task.Id, task.Name)

		originalStatus := task.Status
		task.Status = mapStatus(task.Status, statusMappings)
		fmt.Printf("🔄 Status %s: %s → Status %s: %s\n", source, originalStatus, destination, task.Status)

		destAssignees := make([]models.TaskAssignee, 0, len(task.Assignees))
		for _, a := range task.Assignees {
			if destID, ok := resolvedAssignees[a.ID]; ok {
				destAssignees = append(destAssignees, models.TaskAssignee{ID: destID})
			}
		}
		task.Assignees = destAssignees

		created, err := destClient.CreateTask(destListId, task)
		if err != nil {
			mapping := &repository.TaskMapping{
				MigrationID:  migrationID,
				SourceTaskID: task.Id,
				Status:       "failed",
				ErrorMessage: err.Error(),
			}
			s.taskMappingRepo.Create(mapping)
			fmt.Printf("❌ Erro ao migrar task %s: %v\n", task.Name, err)
			failCount++
			s.migrationRepo.UpdateProgress(migrationID, successCount, failCount)
			continue
		}

		mapping := &repository.TaskMapping{
			MigrationID:  migrationID,
			SourceTaskID: task.Id,
			DestTaskID:   created.Id,
			Status:       "success",
		}
		s.taskMappingRepo.Create(mapping)
		fmt.Printf("✅ Migrada! ID destino: %s\n\n", created.Id)
		successCount++
		s.migrationRepo.UpdateProgress(migrationID, successCount, failCount)
	}

	finalStatus := "completed"
	if failCount > 0 {
		finalStatus = "completed_with_errors"
	}
	s.migrationRepo.Complete(migrationID, finalStatus)
}

func (s *MigrationService) StartMigrationAsync(
	source string,
	destination string,
	sourceProjectId string,
	destListId string,
	destWorkspaceId string,
	statusMappings []models.StatusMapping,
	assigneesMappings []models.AssigneeMapping,
) (int64, error) {
	statusMappingsJSON, err := json.Marshal(statusMappings)
	if err != nil {
		return 0, fmt.Errorf("marshal status mappings: %w", err)
	}

	migration := &repository.Migration{
		Source:           source,
		Destination:      destination,
		SourceProjectID:  sourceProjectId,
		DestListID:       destListId,
		DestWorkspaceID:  destWorkspaceId,
		StatusMappings:   string(statusMappingsJSON),
		Status:           "pending",
		TotalTasks:       0,
	}

	migrationId, err := s.migrationRepo.Create(migration)
	if err != nil {
		return 0, fmt.Errorf("Error trying to create register inside of DB: %w", err)
	}

	var sourceClient, destClient client.TaskClient
	if source == "clickup" {
		sourceClient = s.clickupClient
		destClient = s.asanaClient
	} else {
		sourceClient = s.asanaClient
		destClient = s.clickupClient
	}

	go s.executeMigration(sourceClient, source, destClient, destination, migrationId, sourceProjectId, destListId, destWorkspaceId, statusMappings, assigneesMappings)

	return migrationId, nil
}

func (s *MigrationService) GetPendingAssignees(migrationID int64) ([]repository.PendingAssigneeMapping, []models.Member, error) {
	migration, err := s.migrationRepo.GetMigration(migrationID)
	if err != nil {
		return nil, nil, fmt.Errorf("get migration: %w", err)
	}

	pending, err := s.pendingAssigneeMappingRepo.GetByMigrationID(migrationID)
	if err != nil {
		return nil, nil, fmt.Errorf("get pending assignees: %w", err)
	}

	destMembers, err := s.getMembersForDestination(migration.Destination, migration.DestListID, migration.DestWorkspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("get destination members: %w", err)
	}

	return pending, destMembers, nil
}

func (s *MigrationService) ResumeMigration(migrationID int64, manualMappings []models.AssigneeMapping) error {
	pending, err := s.pendingAssigneeMappingRepo.GetByMigrationID(migrationID)
	if err != nil {
		return fmt.Errorf("get pending assignees: %w", err)
	}

	manualBySource := make(map[string]string)
	for _, m := range manualMappings {
		manualBySource[m.SourceUserId] = m.DestUserId
	}

	for _, p := range pending {
		if _, ok := manualBySource[p.SourceUserId]; !ok {
			return fmt.Errorf("missing mapping for source user %s (%s)", p.SourceUserId, p.SourceUserEmail)
		}
	}

	if err := s.pendingAssigneeMappingRepo.DeleteByMigrationID(migrationID); err != nil {
		return fmt.Errorf("delete pending mappings: %w", err)
	}

	migration, err := s.migrationRepo.GetMigration(migrationID)
	if err != nil {
		return fmt.Errorf("get migration: %w", err)
	}

	var sourceClient, destClient client.TaskClient
	if migration.Source == "clickup" {
		sourceClient = s.clickupClient
	} else {
		sourceClient = s.asanaClient
	}
	if migration.Destination == "clickup" {
		destClient = s.clickupClient
	} else {
		destClient = s.asanaClient
	}

	tasks, err := sourceClient.GetTasks(migration.SourceProjectID)
	if err != nil {
		return fmt.Errorf("get tasks: %w", err)
	}

	destMembers, err := s.getMembersForDestination(migration.Destination, migration.DestListID, migration.DestWorkspaceID)
	if err != nil {
		return fmt.Errorf("get destination members: %w", err)
	}

	emailToDestID := make(map[string]string)
	for _, m := range destMembers {
		if m.Email != "" {
			emailToDestID[m.Email] = m.ID
		}
	}

	resolvedAssignees := make(map[string]string)
	for _, m := range manualMappings {
		resolvedAssignees[m.SourceUserId] = m.DestUserId
	}

	for _, task := range tasks {
		for _, a := range task.Assignees {
			if _, ok := resolvedAssignees[a.ID]; ok {
				continue
			}
			if destID, ok := emailToDestID[a.Email]; ok && a.Email != "" {
				resolvedAssignees[a.ID] = destID
			}
		}
	}

	var statusMappings []models.StatusMapping
	if migration.StatusMappings != "" {
		_ = json.Unmarshal([]byte(migration.StatusMappings), &statusMappings)
	}

	s.migrationRepo.UpdateStatus(migrationID, "running")
	go s.continueExecution(sourceClient, destClient, migration.Source, migration.Destination, migrationID, migration.DestListID, tasks, statusMappings, resolvedAssignees)

	return nil
}

func (s *MigrationService) GetMigrations() ([]repository.Migration, error) {
	migrations, err := s.migrationRepo.GetMigrations()
	if err != nil {
		return nil, fmt.Errorf("Error trying to get registers inside of DB: %w", err)
	}

	return migrations, nil
}

func (s *MigrationService) GetMigration(id int64) (repository.Migration, error) {
	migration, err := s.migrationRepo.GetMigration(id)
	if err != nil {
		return repository.Migration{}, fmt.Errorf("Error trying to get migration: %w", err)
	}

	return migration, nil
}
