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
	SourceProjectId string `json:"source_project_id"` // Asana project GID or ClickUp space GID
	DestListId      string `json:"dest_list_id"`       // Asana project GID when dest=asana
	DestSpaceId     string `json:"dest_space_id"`      // ClickUp space GID when dest=clickup
	DestWorkspaceId string `json:"dest_workspace_id"`
}

type SaveMappingsRequestBody struct {
	Mappings []struct {
		Type        string `json:"type"`
		SourceValue string `json:"source_value"`
		DestValue   string `json:"dest_value"`
	} `json:"mappings"`
	ContainerMappings []struct {
		SourceID string `json:"source_id"`
		DestID   string `json:"dest_id"`
		DestName string `json:"dest_name"`
	} `json:"container_mappings"`
	CustomFieldSelections []struct {
		FieldID string `json:"field_id"`
		Enabled bool   `json:"enabled"`
	} `json:"custom_field_selections"`
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
	if req.Source == "clickup" && req.SourceProjectId == "" {
		writeError(w, http.StatusBadRequest, "source_project_id (space GID) is required for ClickUp source")
		return
	}
	if req.Destination == "clickup" && req.DestSpaceId == "" {
		writeError(w, http.StatusBadRequest, "dest_space_id is required for ClickUp destination")
		return
	}
	if req.Destination == "asana" && req.DestListId == "" {
		writeError(w, http.StatusBadRequest, "dest_list_id (project GID) is required for Asana destination")
		return
	}

	migrationID, state, err := h.migrationService.CreateMigration(service.CreateMigrationInput{
		Source:          req.Source,
		Destination:     req.Destination,
		SourceProjectID: req.SourceProjectId,
		DestListID:      req.DestListId,
		DestWorkspaceID: req.DestWorkspaceId,
		DestSpaceID:     req.DestSpaceId,
	})
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

	containerInputs := make([]service.ContainerMappingInput, 0, len(req.ContainerMappings))
	for _, cm := range req.ContainerMappings {
		if cm.SourceID == "" || cm.DestID == "" {
			writeError(w, http.StatusBadRequest, "container source_id and dest_id are required")
			return
		}
		containerInputs = append(containerInputs, service.ContainerMappingInput{
			SourceID: cm.SourceID,
			DestID:   cm.DestID,
			DestName: cm.DestName,
		})
	}

	cfSelections := make([]service.CustomFieldSelection, 0, len(req.CustomFieldSelections))
	for _, s := range req.CustomFieldSelections {
		cfSelections = append(cfSelections, service.CustomFieldSelection{
			FieldID: s.FieldID,
			Enabled: s.Enabled,
		})
	}

	state, err := h.migrationService.SaveMappings(id, inputs, containerInputs, cfSelections)
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
