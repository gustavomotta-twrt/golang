package client

import (
	"context"

	"github.com/TWRT/integration-mapper/internal/models"
)

// Container represents a groupable unit of tasks: an Asana section or a ClickUp list.
type Container struct {
	ID   string
	Name string
}

type TaskClient interface {
	GetTasks(ctx context.Context, id string) ([]models.Task, error)
	CreateTask(ctx context.Context, id string, workspaceId string, task models.Task) (*models.Task, error)
}

// ContainerProvider is implemented by clients that support container-based (section/list) migration.
type ContainerProvider interface {
	// GetSourceContainers returns the containers of the given top-level project/space.
	// Asana: returns sections of the project. ClickUp: returns lists of the space.
	GetSourceContainers(ctx context.Context, id string) ([]Container, error)
	// GetTasksByContainer returns tasks inside the given container.
	// Asana: tasks in a section. ClickUp: tasks in a list.
	GetTasksByContainer(ctx context.Context, containerId string) ([]models.Task, error)
	// GetDestContainers returns the available destination containers for mapping.
	// Asana: sections of a project. ClickUp: lists of a space.
	GetDestContainers(ctx context.Context, id string) ([]Container, error)
}

type MemberProvider interface {
	GetMembers(ctx context.Context, workspaceId string) ([]models.Member, error)
}

type StatusProvider interface {
	GetListStatuses(ctx context.Context, listId string) ([]string, error)
}

type PriorityLookup interface {
	GetProjectCustomFieldOptions(ctx context.Context, projectGid string) (map[string]string, error)
}

type FieldProvider interface {
	GetFieldDefinitions(ctx context.Context, listId string) ([]models.CustomFieldDefinition, error)
}

type FieldCreator interface {
	CreateCustomField(ctx context.Context, workspaceId, name, asanaType string, options []string) (fieldGID string, optionGIDs []string, err error)
	AttachCustomFieldToProject(ctx context.Context, projectGid, fieldGid string) error
	GetProjectCustomField(ctx context.Context, projectGid, name string) (fieldGID string, optionGIDs []string, found bool, err error)
	FindCustomFieldByName(ctx context.Context, workspaceId, name string) (fieldGID string, optionGIDs []string, err error)
}

type IntegrationProvider interface {
	TaskClient
	MemberProvider
	StatusProvider
}
