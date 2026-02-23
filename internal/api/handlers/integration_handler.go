package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/TWRT/integration-mapper/internal/service"
)

type IntegrationHandler struct {
	integrationService *service.IntegrationService
}

func NewIntegrationHandler(integrationService *service.IntegrationService) *IntegrationHandler {
	return &IntegrationHandler{
		integrationService: integrationService,
	}
}

func (h *IntegrationHandler) GetAsanaWorkspaces(w http.ResponseWriter, r *http.Request) {
	workspaces, err := h.integrationService.GetAsanaWorkspaces()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Error trying to get Asana Workspaces: " + err.Error(),
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"workspaces": workspaces,
	})
}

func (h *IntegrationHandler) GetAsanaProjects(w http.ResponseWriter, r *http.Request) {
	workspace := r.PathValue("id")
	projects, err := h.integrationService.GetAsanaProjects(workspace)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Error trying to get asana projects: " + err.Error(),
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"projects": projects,
	})
}

func (h *IntegrationHandler) GetClickupWorkspaces(w http.ResponseWriter, r *http.Request) {
	workspaces, err := h.integrationService.GetClickupWorkspaces()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Error trying to get ClickUp Workspaces: " + err.Error(),
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"workspaces": workspaces,
	})
}

func (h *IntegrationHandler) GetClickupSpaces(w http.ResponseWriter, r *http.Request) {
	workspaceId := r.PathValue("id")
	spaces, err := h.integrationService.GetClickupSpaces(workspaceId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Error trying to get ClickUp spaces: " + err.Error(),
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"spaces": spaces,
	})
}
