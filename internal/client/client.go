package client

import "github.com/TWRT/integration-mapper/internal/models"

// Container represents a groupable unit of tasks: an Asana section or a ClickUp list.
type Container struct {
	ID   string
	Name string
}

type TaskClient interface {
	GetTasks(id string) ([]models.Task, error)
	CreateTask(id string, workspaceId string, task models.Task) (*models.Task, error)
}

// ContainerProvider is implemented by clients that support container-based (section/list) migration.
type ContainerProvider interface {
	// GetSourceContainers returns the containers of the given top-level project/space.
	// Asana: returns sections of the project. ClickUp: returns lists of the space.
	GetSourceContainers(id string) ([]Container, error)
	// GetTasksByContainer returns tasks inside the given container.
	// Asana: tasks in a section. ClickUp: tasks in a list.
	GetTasksByContainer(containerId string) ([]models.Task, error)
	// GetDestContainers returns the available destination containers for mapping.
	// Asana: sections of a project. ClickUp: lists of a space.
	GetDestContainers(id string) ([]Container, error)
}

type MemberProvider interface {
	GetMembers(workspaceId string) ([]models.Member, error)
}

type StatusProvider interface {
	GetListStatuses(listId string) ([]string, error)
}

type PriorityLookup interface {
	GetProjectCustomFieldOptions(projectGid string) (map[string]string, error)
}

type FieldProvider interface {
	GetFieldDefinitions(listId string) ([]models.CustomFieldDefinition, error)
}

type FieldCreator interface {
	CreateCustomField(workspaceId, name, asanaType string, options []string) (fieldGID string, optionGIDs []string, err error)
	AttachCustomFieldToProject(projectGid, fieldGid string) error
	GetProjectCustomField(projectGid, name string) (fieldGID string, optionGIDs []string, found bool, err error)
	FindCustomFieldByName(workspaceId, name string) (fieldGID string, optionGIDs []string, err error)
}

type IntegrationProvider interface {
	TaskClient
	MemberProvider
	StatusProvider
}
