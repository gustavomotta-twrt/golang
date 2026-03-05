package service

import (
	"github.com/TWRT/integration-mapper/internal/client/asana"
	"github.com/TWRT/integration-mapper/internal/client/clickup"
)

// Internal client interfaces — allow mock injection in tests.

type asanaProvider interface {
	GetWorkspaces() ([]asana.GetMultipleWorkspacesResponse, error)
	GetProjects(workspaceId string) ([]asana.GetMultipleProjectsResponse, error)
	GetSections(projectId string) ([]asana.AsanaSection, error)
}

type clickupProvider interface {
	GetWorkspaces() ([]clickup.ClickUpTeams, error)
	GetSpaces(workspaceId string) ([]clickup.ClickUpSpace, error)
	GetLists(spaceId string) ([]clickup.ClickUpList, error)
	GetListCustomFields(listId string) ([]clickup.ClickUpCustomField, error)
}

type IntegrationService struct {
	asanaClient   asanaProvider
	clickupClient clickupProvider
}

func NewIntegrationService(
	asanaClient asanaProvider,
	clickupClient clickupProvider,
) *IntegrationService {
	return &IntegrationService{
		asanaClient:   asanaClient,
		clickupClient: clickupClient,
	}
}

// IntegrationServiceProvider is the interface consumed by handlers.
// Allows substitution with mocks in tests.
type IntegrationServiceProvider interface {
	GetAsanaWorkspaces() ([]asana.GetMultipleWorkspacesResponse, error)
	GetAsanaProjects(workspaceId string) ([]asana.GetMultipleProjectsResponse, error)
	GetAsanaSections(projectId string) ([]asana.AsanaSection, error)
	GetClickupWorkspaces() ([]clickup.ClickUpTeams, error)
	GetClickupSpaces(workspaceId string) ([]clickup.ClickUpSpace, error)
	GetClickupLists(spaceId string) ([]clickup.ClickUpList, error)
	GetClickupListCustomFields(listId string) ([]clickup.ClickUpCustomField, error)
}

func (s *IntegrationService) GetAsanaWorkspaces() ([]asana.GetMultipleWorkspacesResponse, error) {
	return s.asanaClient.GetWorkspaces()
}

func (s *IntegrationService) GetAsanaProjects(workspaceId string) ([]asana.GetMultipleProjectsResponse, error) {
	return s.asanaClient.GetProjects(workspaceId)
}

func (s *IntegrationService) GetAsanaSections(projectId string) ([]asana.AsanaSection, error) {
	return s.asanaClient.GetSections(projectId)
}

func (s *IntegrationService) GetClickupWorkspaces() ([]clickup.ClickUpTeams, error) {
	return s.clickupClient.GetWorkspaces()
}

func (s *IntegrationService) GetClickupSpaces(workspaceId string) ([]clickup.ClickUpSpace, error) {
	return s.clickupClient.GetSpaces(workspaceId)
}

func (s *IntegrationService) GetClickupLists(spaceId string) ([]clickup.ClickUpList, error) {
	return s.clickupClient.GetLists(spaceId)
}

func (s *IntegrationService) GetClickupListCustomFields(listId string) ([]clickup.ClickUpCustomField, error) {
	return s.clickupClient.GetListCustomFields(listId)
}