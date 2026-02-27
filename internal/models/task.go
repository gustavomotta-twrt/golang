package models

import "time"

type TaskAssignee struct {
	ID    string
	Name  string
	Email string
}

type CustomFieldOption struct {
	ID         string
	Name       string
	OrderIndex int
}

type CustomFieldDefinition struct {
	ID          string
	Name        string
	ClickUpType string
	Options     []CustomFieldOption
}

type TaskCustomField struct {
	FieldID string
	Value   interface{}
}

type Task struct {
	Id           string
	Name         string
	Description  string
	Status       string
	Completed    bool
	Assignees    []TaskAssignee
	DueDate      *time.Time
	Tags         []string
	Priority     string
	CustomFields []TaskCustomField
}
