package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/TWRT/integration-mapper/internal/repository"
	"github.com/TWRT/integration-mapper/internal/service"
)

type MigrationHandler struct {
	migrationService *service.MigrationService
}

func NewMigrationHandler(migrationService *service.MigrationService) *MigrationHandler {
	return &MigrationHandler{migrationService: migrationService}
}

type CreateMigrationRequestBody struct {
	Source          string `json:"source"`
	Destination     string `json:"destination"`
	SourceProjectId string `json:"source_project_id"`
	DestListId      string `json:"dest_list_id"`
	DestWorkspaceId string `json:"dest_workspace_id"`
}

type SaveMappingsRequestBody struct {
	Mappings []struct {
		Type        string `json:"type"`
		SourceValue string `json:"source_value"`
		DestValue   string `json:"dest_value"`
	} `json:"mappings"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func parseMigrationID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func readBody(r *http.Request) ([]byte, error) {
	return io.ReadAll(r.Body)
}

func (h *MigrationHandler) CreateMigration(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error reading body: "+err.Error())
		return
	}

	var req CreateMigrationRequestBody
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.Source != "asana" && req.Source != "clickup" {
		writeError(w, http.StatusBadRequest, "source must be 'asana' or 'clickup'")
		return
	}
	if req.Destination != "asana" && req.Destination != "clickup" {
		writeError(w, http.StatusBadRequest, "destination must be 'asana' or 'clickup'")
		return
	}
	if req.Source == req.Destination {
		writeError(w, http.StatusBadRequest, "source and destination must be different")
		return
	}
	if req.DestWorkspaceId == "" {
		writeError(w, http.StatusBadRequest, "dest_workspace_id is required")
		return
	}

	migrationID, state, err := h.migrationService.CreateMigration(
		req.Source,
		req.Destination,
		req.SourceProjectId,
		req.DestListId,
		req.DestWorkspaceId,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error creating migration: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"migration_id": migrationID,
		"status":       "pending_configuration",
		"mappings":     state,
	})
}

func (h *MigrationHandler) GetMappings(w http.ResponseWriter, r *http.Request) {
	id, err := parseMigrationID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid migration id")
		return
	}

	state, err := h.migrationService.SyncMappings(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error syncing mappings: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"mappings": state,
	})
}

func (h *MigrationHandler) SaveMappings(w http.ResponseWriter, r *http.Request) {
	id, err := parseMigrationID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid migration id")
		return
	}

	body, err := readBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error reading body: "+err.Error())
		return
	}

	var req SaveMappingsRequestBody
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if len(req.Mappings) == 0 {
		writeError(w, http.StatusBadRequest, "mappings cannot be empty")
		return
	}

	inputs := make([]service.MappingInput, 0, len(req.Mappings))
	for _, m := range req.Mappings {
		if m.SourceValue == "" || m.DestValue == "" {
			writeError(w, http.StatusBadRequest, "source_value and dest_value are required")
			return
		}
		inputs = append(inputs, service.MappingInput{
			Type:        repository.MappingType(m.Type),
			SourceValue: m.SourceValue,
			DestValue:   m.DestValue,
		})
	}

	state, err := h.migrationService.SaveMappings(id, inputs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error saving mappings: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"mappings": state,
	})
}

func (h *MigrationHandler) StartMigration(w http.ResponseWriter, r *http.Request) {
	id, err := parseMigrationID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid migration id")
		return
	}

	if err := h.migrationService.StartMigration(id); err != nil {
		writeError(w, http.StatusBadRequest, "error starting migration: "+err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"migration_id": id,
		"status":       "running",
		"message":      "Migration started successfully",
	})
}

func (h *MigrationHandler) GetMigration(w http.ResponseWriter, r *http.Request) {
	id, err := parseMigrationID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid migration id")
		return
	}

	migration, err := h.migrationService.GetMigration(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error getting migration: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"migration": migration,
	})
}

func (h *MigrationHandler) ListMigrations(w http.ResponseWriter, r *http.Request) {
	migrations, err := h.migrationService.GetMigrations()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error listing migrations: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"migrations": migrations,
	})
}
