package handlers

import (
	"io"
	"net/http"
	"encoding/json"

	"github.com/TWRT/integration-mapper/internal/service"
	"github.com/TWRT/integration-mapper/internal/models"
)

type CreateMigrationRequestBody struct {
	Source           string             `json:"source"`
	Destination      string             `json:"destination"`
	SourceProjectId  string             `json:"source_project_id"`
	DestListId       string             `json:"dest_list_id"`
	StatusMappings   []models.StatusMapping   `json:"status_mappings"`
	AssigneeMappings []models.AssigneeMapping `json:"assignee_mappings"`
}

type MigrationHandler struct {
	migrationService *service.MigrationService
}

func NewMigrationHandler(migrationService *service.MigrationService) *MigrationHandler {
	return &MigrationHandler{
		migrationService: migrationService,
	}
}

func (h *MigrationHandler) CreateMigration(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Error trying to read the body: " + err.Error(),
		})
		return
	}

	var reqBody CreateMigrationRequestBody
	if err := json.Unmarshal(body, &reqBody); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "JSON error: " + err.Error(),
		})
		return
	}

	migrationId, err := h.migrationService.StartMigrationAsync(
		reqBody.SourceProjectId,
		reqBody.DestListId,
		reqBody.StatusMappings,
		reqBody.AssigneeMappings,
	)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Error trying to start migration: " + err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusAccepted)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"migration_id": migrationId,
		"status":       "pending",
		"message":      "Migration initiated successfully",
	})
}

func (h *MigrationHandler) GetMigration(w http.ResponseWriter, r *http.Request) {

}

func (h *MigrationHandler) ListMigrations(w http.ResponseWriter, r *http.Request) {

}
