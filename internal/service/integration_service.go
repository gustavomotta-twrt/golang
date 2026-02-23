package service

import (
	"github.com/TWRT/integration-mapper/internal/client/asana"
	"github.com/TWRT/integration-mapper/internal/client/clickup"
)

type IntegrationService struct {
	asanaClient   *asana.AsanaClient
	clickupClient *clickup.ClickUpClient
}

func NewIntegrationService(
	asanaClient *asana.AsanaClient,
	clickupClient *clickup.ClickUpClient,
) *IntegrationService {
	return &IntegrationService{
		asanaClient:   asanaClient,
		clickupClient: clickupClient,
	}
}

func (s *IntegrationService) GetAsanaWorkspaces() ([]asana.GetMultipleWorkspacesResponse, error) {
	return s.asanaClient.GetWorkspaces()
}

func (s *IntegrationService) GetAsanaProjects(workspaceId string) ([]asana.GetMultipleProjectsResponse, error) {
	return s.asanaClient.GetProjects(workspaceId)
}

func (s *IntegrationService) GetClickupWorkspaces() ([]clickup.ClickUpTeams, error) {
	return s.clickupClient.GetWorkspaces()
}

func (s *IntegrationService) GetClickupSpaces(workspaceId string) ([]clickup.ClickUpSpace, error) {
	return s.clickupClient.GetSpaces(workspaceId)
}