package service

import (
	"context"

	"github.com/TWRT/integration-mapper/internal/client/asana"
	"github.com/TWRT/integration-mapper/internal/client/clickup"
)

// Internal client interfaces — allow mock injection in tests.

type asanaProvider interface {
	GetWorkspaces(ctx context.Context) ([]asana.GetMultipleWorkspacesResponse, error)
	GetProjects(ctx context.Context, workspaceId string) ([]asana.GetMultipleProjectsResponse, error)
	GetSections(ctx context.Context, projectId string) ([]asana.AsanaSection, error)
}

type clickupProvider interface {
	GetWorkspaces(ctx context.Context) ([]clickup.ClickUpTeams, error)
	GetSpaces(ctx context.Context, workspaceId string) ([]clickup.ClickUpSpace, error)
	GetLists(ctx context.Context, spaceId string) ([]clickup.ClickUpList, error)
	GetListCustomFields(ctx context.Context, listId string) ([]clickup.ClickUpCustomField, error)
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
	GetAsanaWorkspaces(ctx context.Context) ([]asana.GetMultipleWorkspacesResponse, error)
	GetAsanaProjects(ctx context.Context, workspaceId string) ([]asana.GetMultipleProjectsResponse, error)
	GetAsanaSections(ctx context.Context, projectId string) ([]asana.AsanaSection, error)
	GetClickupWorkspaces(ctx context.Context) ([]clickup.ClickUpTeams, error)
	GetClickupSpaces(ctx context.Context, workspaceId string) ([]clickup.ClickUpSpace, error)
	GetClickupLists(ctx context.Context, spaceId string) ([]clickup.ClickUpList, error)
	GetClickupListCustomFields(ctx context.Context, listId string) ([]clickup.ClickUpCustomField, error)
}

func (s *IntegrationService) GetAsanaWorkspaces(ctx context.Context) ([]asana.GetMultipleWorkspacesResponse, error) {
	return s.asanaClient.GetWorkspaces(ctx)
}

func (s *IntegrationService) GetAsanaProjects(ctx context.Context, workspaceId string) ([]asana.GetMultipleProjectsResponse, error) {
	return s.asanaClient.GetProjects(ctx, workspaceId)
}

func (s *IntegrationService) GetAsanaSections(ctx context.Context, projectId string) ([]asana.AsanaSection, error) {
	return s.asanaClient.GetSections(ctx, projectId)
}

func (s *IntegrationService) GetClickupWorkspaces(ctx context.Context) ([]clickup.ClickUpTeams, error) {
	return s.clickupClient.GetWorkspaces(ctx)
}

func (s *IntegrationService) GetClickupSpaces(ctx context.Context, workspaceId string) ([]clickup.ClickUpSpace, error) {
	return s.clickupClient.GetSpaces(ctx, workspaceId)
}

func (s *IntegrationService) GetClickupLists(ctx context.Context, spaceId string) ([]clickup.ClickUpList, error) {
	return s.clickupClient.GetLists(ctx, spaceId)
}

func (s *IntegrationService) GetClickupListCustomFields(ctx context.Context, listId string) ([]clickup.ClickUpCustomField, error) {
	return s.clickupClient.GetListCustomFields(ctx, listId)
}
