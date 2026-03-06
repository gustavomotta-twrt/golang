package handlers

import (
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
		writeError(w, http.StatusInternalServerError, "Error trying to get Asana Workspaces: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaces": workspaces})
}

func (h *IntegrationHandler) GetAsanaProjects(w http.ResponseWriter, r *http.Request) {
	workspace := r.PathValue("id")
	projects, err := h.integrationService.GetAsanaProjects(r.Context(), workspace)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Error trying to get asana projects: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (h *IntegrationHandler) GetAsanaSections(w http.ResponseWriter, r *http.Request) {
	projectId := r.PathValue("id")
	sections, err := h.integrationService.GetAsanaSections(r.Context(), projectId)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Error trying to get Asana sections: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sections": sections})
}

func (h *IntegrationHandler) GetClickupWorkspaces(w http.ResponseWriter, r *http.Request) {
	workspaces, err := h.integrationService.GetClickupWorkspaces(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Error trying to get ClickUp Workspaces: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaces": workspaces})
}

func (h *IntegrationHandler) GetClickupSpaces(w http.ResponseWriter, r *http.Request) {
	workspaceId := r.PathValue("id")
	spaces, err := h.integrationService.GetClickupSpaces(r.Context(), workspaceId)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Error trying to get ClickUp spaces: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"spaces": spaces})
}

func (h *IntegrationHandler) GetClickupLists(w http.ResponseWriter, r *http.Request) {
	spaceId := r.PathValue("id")
	lists, err := h.integrationService.GetClickupLists(r.Context(), spaceId)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Error trying to get ClickUp lists: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lists": lists})
}

func (h *IntegrationHandler) GetClickupListCustomFields(w http.ResponseWriter, r *http.Request) {
	listId := r.PathValue("id")
	fields, err := h.integrationService.GetClickupListCustomFields(r.Context(), listId)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Error trying to get ClickUp list custom fields: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"fields": fields})
}
