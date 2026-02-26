package models

import "time"

type TaskAssignee struct {
	ID    string
	Name  string
	Email string
}

type Task struct {
	Id          string
	Name        string
	Description string
	Status      string
	Completed   bool
	Assignees   []TaskAssignee
	DueDate     *time.Time
	Tags         []string
	Priority     string
	CustomFields []CustomFieldValue
}
