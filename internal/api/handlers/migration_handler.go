package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/TWRT/integration-mapper/internal/repository"
	"github.com/TWRT/integration-mapper/internal/service"
)

type MigrationHandler struct {
	migrationService service.MigrationServiceProvider
}

func NewMigrationHandler(migrationService service.MigrationServiceProvider) *MigrationHandler {
	return &MigrationHandler{migrationService: migrationService}
}

type CreateMigrationRequestBody struct {
	Source          string `json:"source"`
	Destination     string `json:"destination"`
	SourceProjectId string `json:"source_project_id"`
	DestListId      string `json:"dest_list_id"`
	DestSpaceId     string `json:"dest_space_id"`
	DestWorkspaceId string `json:"dest_workspace_id"`
}

// SaveMappingsRequestBody is the new per-container mapping format.
type SaveMappingsRequestBody struct {
	Assignees []struct {
		SourceValue string `json:"source_value"`
		DestValue   string `json:"dest_value"`
	} `json:"assignees"`
	ContainerMappings []struct {
		SourceID         string  `json:"source_id"`
		DestID           *string `json:"dest_id"`
		DestName         *string `json:"dest_name"`
		Enabled          bool    `json:"enabled"`
		StatusMappings   []struct {
			SourceValue string `json:"source_value"`
			DestValue   string `json:"dest_value"`
		} `json:"status_mappings"`
		PriorityMappings []struct {
			SourceValue string `json:"source_value"`
			DestValue   string `json:"dest_value"`
		} `json:"priority_mappings"`
		CustomFields []struct {
			FieldID string `json:"field_id"`
			Enabled bool   `json:"enabled"`
		} `json:"custom_fields"`
	} `json:"container_mappings"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func parseMigrationID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

const maxBodySize = 1 << 20 // 1 MB

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	return io.ReadAll(r.Body)
}

func (h *MigrationHandler) CreateMigration(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(w, r)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid request body")
		}
		return
	}

	var req CreateMigrationRequestBody
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request format")
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
	if req.Destination == "clickup" && req.DestSpaceId == "" {
		writeError(w, http.StatusBadRequest, "dest_space_id is required for ClickUp destination")
		return
	}
	if req.Destination == "asana" && req.DestListId == "" {
		writeError(w, http.StatusBadRequest, "dest_list_id (project GID) is required for Asana destination")
		return
	}

	migrationID, state, err := h.migrationService.CreateMigration(r.Context(), service.CreateMigrationInput{
		Source:          req.Source,
		Destination:     req.Destination,
		SourceProjectID: req.SourceProjectId,
		DestListID:      req.DestListId,
		DestWorkspaceID: req.DestWorkspaceId,
		DestSpaceID:     req.DestSpaceId,
	})
	if err != nil {
		slog.Error("failed to create migration", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create migration")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"migration_id": migrationID,
		"status":       repository.MigrationStatusPendingConfiguration,
		"mappings":     state,
	})
}

func (h *MigrationHandler) GetMappings(w http.ResponseWriter, r *http.Request) {
	id, err := parseMigrationID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid migration id")
		return
	}

	state, err := h.migrationService.SyncMappings(r.Context(), id)
	if err != nil {
		slog.Error("failed to sync mappings", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to sync mappings")
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

	body, err := readBody(w, r)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid request body")
		}
		return
	}

	var req SaveMappingsRequestBody
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request format")
		return
	}

	assignees := make([]service.AssigneeMappingInput, 0, len(req.Assignees))
	for _, a := range req.Assignees {
		assignees = append(assignees, service.AssigneeMappingInput{
			SourceValue: a.SourceValue,
			DestValue:   a.DestValue,
		})
	}

	containerInputs := make([]service.ContainerMappingInput, 0, len(req.ContainerMappings))
	for _, cm := range req.ContainerMappings {
		if cm.SourceID == "" {
			writeError(w, http.StatusBadRequest, "container source_id is required")
			return
		}
		if cm.Enabled && (cm.DestID == nil || *cm.DestID == "") {
			writeError(w, http.StatusBadRequest, "container dest_id is required when enabled")
			return
		}

		statusMappings := make([]service.FieldMappingInput, 0, len(cm.StatusMappings))
		for _, sm := range cm.StatusMappings {
			statusMappings = append(statusMappings, service.FieldMappingInput{
				SourceValue: sm.SourceValue,
				DestValue:   sm.DestValue,
			})
		}

		priorityMappings := make([]service.FieldMappingInput, 0, len(cm.PriorityMappings))
		for _, pm := range cm.PriorityMappings {
			priorityMappings = append(priorityMappings, service.FieldMappingInput{
				SourceValue: pm.SourceValue,
				DestValue:   pm.DestValue,
			})
		}

		customFields := make([]service.CustomFieldSelection, 0, len(cm.CustomFields))
		for _, cf := range cm.CustomFields {
			customFields = append(customFields, service.CustomFieldSelection{
				FieldID: cf.FieldID,
				Enabled: cf.Enabled,
			})
		}

		containerInputs = append(containerInputs, service.ContainerMappingInput{
			SourceID:         cm.SourceID,
			DestID:           cm.DestID,
			DestName:         cm.DestName,
			Enabled:          cm.Enabled,
			StatusMappings:   statusMappings,
			PriorityMappings: priorityMappings,
			CustomFields:     customFields,
		})
	}

	state, err := h.migrationService.SaveMappings(r.Context(), id, assignees, containerInputs)
	if err != nil {
		slog.Error("failed to save mappings", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to save mappings")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"mappings": state,
	})
}

// GetDestContainerOptions returns available statuses and priorities for a given destination container.
func (h *MigrationHandler) GetDestContainerOptions(w http.ResponseWriter, r *http.Request) {
	id, err := parseMigrationID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid migration id")
		return
	}

	destContainerID := r.URL.Query().Get("dest_container_id")
	if destContainerID == "" {
		writeError(w, http.StatusBadRequest, "dest_container_id query param is required")
		return
	}

	statuses, priorities, err := h.migrationService.GetDestContainerOptions(r.Context(), id, destContainerID)
	if err != nil {
		slog.Error("failed to get destination container options", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get destination container options")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"statuses":   statuses,
		"priorities": priorities,
	})
}

func (h *MigrationHandler) StartMigration(w http.ResponseWriter, r *http.Request) {
	id, err := parseMigrationID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid migration id")
		return
	}

	if err := h.migrationService.StartMigration(id); err != nil {
		slog.Error("failed to start migration", "migration_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to start migration")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"migration_id": id,
		"status":       repository.MigrationStatusRunning,
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
		slog.Error("failed to get migration", "migration_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get migration")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"migration": migration,
	})
}

func (h *MigrationHandler) ListMigrations(w http.ResponseWriter, r *http.Request) {
	migrations, err := h.migrationService.GetMigrations()
	if err != nil {
		slog.Error("failed to list migrations", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list migrations")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"migrations": migrations,
	})
}
