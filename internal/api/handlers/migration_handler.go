package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/TWRT/integration-mapper/internal/models"
	"github.com/TWRT/integration-mapper/internal/service"
)

type CreateMigrationRequestBody struct {
	Source           string                   `json:"source"`
	Destination      string                   `json:"destination"`
	SourceProjectId  string                   `json:"source_project_id"`
	DestListId       string                   `json:"dest_list_id"`
	DestWorkspaceId  string                   `json:"dest_workspace_id,omitempty"`
	StatusMappings   []models.StatusMapping   `json:"status_mappings"`
	AssigneeMappings []models.AssigneeMapping `json:"assignee_mappings"`
}

type SubmitAssigneeMappingsRequestBody struct {
	Mappings []models.AssigneeMapping `json:"mappings"`
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Error trying to read the body: " + err.Error(),
		})
		return
	}

	var reqBody CreateMigrationRequestBody
	if err := json.Unmarshal(body, &reqBody); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "JSON error: " + err.Error(),
		})
		return
	}

	if reqBody.Source != "asana" && reqBody.Source != "clickup" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "source must be 'asana' or 'clickup'",
		})
		return
	}
	if reqBody.Destination != "asana" && reqBody.Destination != "clickup" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "destination must be 'asana' or 'clickup'",
		})
		return
	}

	if reqBody.Destination == "clickup" && reqBody.DestWorkspaceId == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "dest_workspace_id is required when destination is clickup",
		})
		return
	}
	if reqBody.Destination == "asana" && reqBody.DestWorkspaceId == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "dest_workspace_id is required when destination is asana",
		})
		return
	}

	migrationId, err := h.migrationService.StartMigrationAsync(
		reqBody.Source,
		reqBody.Destination,
		reqBody.SourceProjectId,
		reqBody.DestListId,
		reqBody.DestWorkspaceId,
		reqBody.StatusMappings,
		reqBody.AssigneeMappings,
	)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Error trying to start migration: " + err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"migration_id": migrationId,
		"status":       "pending",
		"message":      "Migration initiated successfully",
	})
}

func (h *MigrationHandler) GetPendingAssignees(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid migration id",
		})
		return
	}

	pending, members, err := h.migrationService.GetPendingAssignees(id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Error trying to get pending assignees: " + err.Error(),
		})
		return
	}

	pendingResponse := make([]map[string]string, 0, len(pending))
	for _, p := range pending {
		pendingResponse = append(pendingResponse, map[string]string{
			"source_user_id":    p.SourceUserId,
			"source_user_name":  p.SourceUserName,
			"source_user_email": p.SourceUserEmail,
		})
	}

	membersResponse := make([]map[string]string, 0, len(members))
	for _, m := range members {
		membersResponse = append(membersResponse, map[string]string{
			"id":    m.ID,
			"name":  m.Name,
			"email": m.Email,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"pending_assignees":                pendingResponse,
		"available_destination_members": membersResponse,
	})
}

func (h *MigrationHandler) SubmitAssigneeMappings(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid migration id",
		})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Error trying to read the body: " + err.Error(),
		})
		return
	}

	var reqBody SubmitAssigneeMappingsRequestBody
	if err := json.Unmarshal(body, &reqBody); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "JSON error: " + err.Error(),
		})
		return
	}

	if err := h.migrationService.ResumeMigration(id, reqBody.Mappings); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Error trying to resume migration: " + err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"migration_id": id,
		"status":       "running",
		"message":      "Migration resumed successfully",
	})
}

func (h *MigrationHandler) GetMigration(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	migration, err := h.migrationService.GetMigration(id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Error trying to get migration: " + err.Error(),
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"migration": migration,
	})
}

func (h *MigrationHandler) ListMigrations(w http.ResponseWriter, r *http.Request) {
	migrations, err := h.migrationService.GetMigrations()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Error trying to get migrations: " + err.Error(),
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"migrations": migrations,
	})
}
