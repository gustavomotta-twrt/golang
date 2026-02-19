package client

import "github.com/TWRT/integration-mapper/internal/models"

type TaskClient interface {
	GetTasks(id string) ([]models.Task, error)
	CreateTask(id string, task models.Task) (*models.Task, error)
}
