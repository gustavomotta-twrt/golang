package service

import (
	"fmt"

	"github.com/TWRT/integration-mapper/internal/client"
	"github.com/TWRT/integration-mapper/internal/models"
	"github.com/TWRT/integration-mapper/internal/repository"
)

type MigrationService struct {
	asanaClient     client.TaskClient
	clickupClient   client.TaskClient
	migrationRepo   *repository.MigrationRepository
	taskMappingRepo *repository.TaskMappingRepository
}

func NewMigrationService(
	asanaClient,
	clickupClient client.TaskClient,
	migrationRepo *repository.MigrationRepository,
	taskMappingRepo *repository.TaskMappingRepository,
) *MigrationService {
	return &MigrationService{
		asanaClient:     asanaClient,
		clickupClient:   clickupClient,
		migrationRepo:   migrationRepo,
		taskMappingRepo: taskMappingRepo,
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
	sourceClient client.TaskClient,
	source string,
	destClient client.TaskClient,
	destination string,
	migrationID int64,
	sourceProjectId string,
	destListId string,
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

	successCount := 0
	failCount := 0

	for _, task := range tasks {
		fmt.Printf("⏳ Migrando: [%s] %s...\n", task.Id, task.Name)

		originalStatus := task.Status
		task.Status = mapStatus(task.Status, statusMappings)
		fmt.Printf("🔄 Status %s: %s → Status %s: %s\n", source, originalStatus, destination, task.Status)

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
	statusMappings []models.StatusMapping,
	assigneesMappings []models.AssigneeMapping,
) (int64, error) {
	migration := &repository.Migration{
		Source:          source,
		Destination:     destination,
		SourceProjectID: sourceProjectId,
		DestListID:      destListId,
		Status:          "pending",
		TotalTasks:      0,
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

	go s.executeMigration(sourceClient, source, destClient, destination, migrationId, sourceProjectId, destListId, statusMappings, assigneesMappings)

	return migrationId, nil
}

func (s *MigrationService) GetMigrations() ([]repository.Migration, error) {
	migrations, err := s.migrationRepo.GetMigrations()
	if err != nil {
		return nil, fmt.Errorf("Error trying to get registers inside of DB: %w", err)
	}

	return migrations, nil
}

func (s *MigrationService) GetMigration(id string) (repository.Migration, error) {
	migration, err := s.migrationRepo.GetMigration(id)
	if err != nil {
		return repository.Migration{}, fmt.Errorf("Error trying to get the migration: %w", err)
	}

	return migration, nil
}
