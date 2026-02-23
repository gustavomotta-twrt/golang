package api

import (
	"database/sql"
	"net/http"

	"github.com/TWRT/integration-mapper/internal/api/handlers"
	"github.com/TWRT/integration-mapper/internal/client/asana"
	"github.com/TWRT/integration-mapper/internal/client/clickup"
	"github.com/TWRT/integration-mapper/internal/repository"
	"github.com/TWRT/integration-mapper/internal/service"
)

func SetupRouter(db *sql.DB, asanaToken string, clickupToken string) *http.ServeMux {
	mux := http.NewServeMux()

	asanaClient := asana.NewAsanaClient(asanaToken)
	clickUpClient := clickup.NewClickUpClient(clickupToken)

	migrationRepo := repository.NewMigrationRepository(db)
	taskMappingRepo := repository.NewTaskMappingRepository(db)
	pendingAssigneeMappingRepo := repository.NewPendingAssigneeMappingRepository(db)

	migrationService := service.NewMigrationService(
		asanaClient,
		clickUpClient,
		asanaClient,
		clickUpClient,
		migrationRepo,
		taskMappingRepo,
		pendingAssigneeMappingRepo,
	)

	integrationService := service.NewIntegrationService(
		asanaClient,
		clickUpClient,
	)

	migrationHandler := handlers.NewMigrationHandler(migrationService)
	integrationHandler := handlers.NewIntegrationHandler(integrationService)

	mux.HandleFunc("POST /migrations", migrationHandler.CreateMigration)
	mux.HandleFunc("GET /migrations/{id}", migrationHandler.GetMigration)
	mux.HandleFunc("GET /migrations", migrationHandler.ListMigrations)
	mux.HandleFunc("GET /asana/workspaces", integrationHandler.GetAsanaWorkspaces)
	mux.HandleFunc("GET /asana/workspaces/{id}/projects", integrationHandler.GetAsanaProjects)
	mux.HandleFunc("GET /clickup/workspaces", integrationHandler.GetClickupWorkspaces)
	mux.HandleFunc("GET /clickup/workspaces/{id}/spaces", integrationHandler.GetClickupSpaces)
	mux.HandleFunc("GET /migrations/{id}/pending-assignees", migrationHandler.GetPendingAssignees)
	mux.HandleFunc("POST /migrations/{id}/assignee-mappings", migrationHandler.SubmitAssigneeMappings)

	return mux
}
