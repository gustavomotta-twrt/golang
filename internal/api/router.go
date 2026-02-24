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
	migrationMappingRepo := repository.NewMigrationMappingRepository(db)

	migrationService := service.NewMigrationService(
		asanaClient,
		clickUpClient,
		asanaClient,
		clickUpClient,
		migrationRepo,
		taskMappingRepo,
		migrationMappingRepo,
	)

	integrationService := service.NewIntegrationService(
		asanaClient,
		clickUpClient,
	)

	migrationHandler := handlers.NewMigrationHandler(migrationService)
	integrationHandler := handlers.NewIntegrationHandler(integrationService)

	mux.HandleFunc("POST /migrations/create", migrationHandler.CreateMigration)
	mux.HandleFunc("GET /migrations/{id}/mappings", migrationHandler.GetMappings)
	mux.HandleFunc("POST /migrations/{id}/mappings", migrationHandler.SaveMappings)
	mux.HandleFunc("POST /migrations/{id}/start", migrationHandler.StartMigration)
	mux.HandleFunc("GET /migrations/{id}", migrationHandler.GetMigration)
	mux.HandleFunc("GET /migrations", migrationHandler.ListMigrations)

	mux.HandleFunc("GET /asana/workspaces", integrationHandler.GetAsanaWorkspaces)
	mux.HandleFunc("GET /asana/workspaces/{id}/projects", integrationHandler.GetAsanaProjects)
	mux.HandleFunc("GET /clickup/workspaces", integrationHandler.GetClickupWorkspaces)
	mux.HandleFunc("GET /clickup/workspaces/{id}/spaces", integrationHandler.GetClickupSpaces)

	return mux
}
