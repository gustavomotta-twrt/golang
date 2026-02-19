package service

import (
	"fmt"

	"github.com/TWRT/integration-mapper/internal/client"
	"github.com/TWRT/integration-mapper/internal/models"
	"github.com/TWRT/integration-mapper/internal/repository"
)

type MigrationService struct {
	sourceClient      client.TaskClient
	destinationClient client.TaskClient
	migrationRepo     *repository.MigrationRepository
	taskMappingRepo   *repository.TaskMappingRepository
}

func NewMigrationService(
	source,
	destination client.TaskClient,
	migrationRepo *repository.MigrationRepository,
	taskMappingRepo *repository.TaskMappingRepository,
) *MigrationService {
	return &MigrationService{
		sourceClient:      source,
		destinationClient: destination,
		migrationRepo:     migrationRepo,
		taskMappingRepo:   taskMappingRepo,
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

func (s *MigrationService) executeMigration(
	migrationID int64,
	sourceProjectId string,
	destListId string,
	statusMappings []models.StatusMapping,
	assigneeMappings []models.AssigneeMapping,
) {
	tasks, err := s.sourceClient.GetTasks(sourceProjectId)
	if err != nil {
		s.migrationRepo.Complete(migrationID, "failed")
		fmt.Printf("❌ Erro ao buscar tasks: %v\n", err)
		return
	}

	// Passo 2: Atualizar total de tasks no BD
	// TODO: Criar método UpdateTotal no repository

	// Passo 3: Atualizar status para "running"
	// TODO: Criar método UpdateStatus no repository

	// Passo 4: Loop para migrar cada task
	successCount := 0
	failCount := 0

	for _, task := range tasks {
		fmt.Printf("⏳ Migrando: [%s] %s...\n", task.Id, task.Name)

		fmt.Printf("STATUS ASANA: %s\n", task.Status)
		task.Status = mapStatus(task.Status, statusMappings)
		fmt.Printf("STATUS APÓS MAP: %s\n", task.Status)
		
		created, err := s.destinationClient.CreateTask(destListId, task)
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
	sourceProjectId string,
	destListId string,
	statusMappings []models.StatusMapping,
	assigneesMappings []models.AssigneeMapping,
) (int64, error) {
	migration := &repository.Migration{
		Source:          "asana",
		Destination:     "clickup",
		SourceProjectID: sourceProjectId,
		DestListID:      destListId,
		Status:          "pending",
		TotalTasks:      0,
	}

	migrationId, err := s.migrationRepo.Create(migration)
	if err != nil {
		return 0, fmt.Errorf("Error trying to create register inside of DB: %w", err)
	}

	go s.executeMigration(migrationId, sourceProjectId, destListId, statusMappings, assigneesMappings)

	return migrationId, nil
}
