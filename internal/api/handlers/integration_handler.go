package handlers

import (
	"log/slog"
	"net/http"

	"github.com/TWRT/integration-mapper/internal/service"
)

type IntegrationHandler struct {
	integrationService service.IntegrationServiceProvider
}

func NewIntegrationHandler(integrationService service.IntegrationServiceProvider) *IntegrationHandler {
	return &IntegrationHandler{
		integrationService: integrationService,
	}
}

func (h *IntegrationHandler) GetAsanaWorkspaces(w http.ResponseWriter, r *http.Request) {
	workspaces, err := h.integrationService.GetAsanaWorkspaces(r.Context())
	if err != nil {
		slog.Error("failed to get asana workspaces", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get asana workspaces")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaces": workspaces})
}

func (h *IntegrationHandler) GetAsanaProjects(w http.ResponseWriter, r *http.Request) {
	workspace := r.PathValue("id")
	projects, err := h.integrationService.GetAsanaProjects(r.Context(), workspace)
	if err != nil {
		slog.Error("failed to get asana projects", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get asana projects")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (h *IntegrationHandler) GetAsanaSections(w http.ResponseWriter, r *http.Request) {
	projectId := r.PathValue("id")
	sections, err := h.integrationService.GetAsanaSections(r.Context(), projectId)
	if err != nil {
		slog.Error("failed to get asana sections", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get asana sections")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sections": sections})
}

func (h *IntegrationHandler) GetClickupWorkspaces(w http.ResponseWriter, r *http.Request) {
	workspaces, err := h.integrationService.GetClickupWorkspaces(r.Context())
	if err != nil {
		slog.Error("failed to get clickup workspaces", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get clickup workspaces")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaces": workspaces})
}

func (h *IntegrationHandler) GetClickupSpaces(w http.ResponseWriter, r *http.Request) {
	workspaceId := r.PathValue("id")
	spaces, err := h.integrationService.GetClickupSpaces(r.Context(), workspaceId)
	if err != nil {
		slog.Error("failed to get clickup spaces", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get clickup spaces")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"spaces": spaces})
}

func (h *IntegrationHandler) GetClickupLists(w http.ResponseWriter, r *http.Request) {
	spaceId := r.PathValue("id")
	lists, err := h.integrationService.GetClickupLists(r.Context(), spaceId)
	if err != nil {
		slog.Error("failed to get clickup lists", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get clickup lists")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lists": lists})
}

func (h *IntegrationHandler) GetClickupListCustomFields(w http.ResponseWriter, r *http.Request) {
	listId := r.PathValue("id")
	fields, err := h.integrationService.GetClickupListCustomFields(r.Context(), listId)
	if err != nil {
		slog.Error("failed to get clickup list fields", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get clickup list fields")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"fields": fields})
}
