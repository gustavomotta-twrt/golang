package client

import "github.com/TWRT/integration-mapper/internal/models"

type TaskClient interface {
	GetTasks(id string) ([]models.Task, error)
	CreateTask(id string, task models.Task) (*models.Task, error)
}

type MemberProvider interface {
	GetMembers(workspaceId string) ([]models.Member, error)
}

type MigrationClient interface {
	TaskClient
	MemberProvider
}
