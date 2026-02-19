package models

import "time"

type Task struct {
	Id          string
	Name        string
	Description string
	Status      string
	Completed   bool
	Assignee    string
	DueDate     *time.Time
	Tags        []string
	Priority    string
}
