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

	migrationService := service.NewMigrationService(
		asanaClient,
		clickUpClient,
		migrationRepo,
		taskMappingRepo,
	)

	migrationHandler := handlers.NewMigrationHandler(migrationService)

	mux.HandleFunc("POST /migrations", migrationHandler.CreateMigration)
	mux.HandleFunc("GET /migrations/{id}", migrationHandler.GetMigration)
	mux.HandleFunc("GET /migrations", migrationHandler.ListMigrations)

	return mux
}
