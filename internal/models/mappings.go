package models

type StatusMapping struct {
	SourceStatus string `json:"source_status"`
	DestStatus   string `json:"dest_status"`
}

type AssigneeMapping struct {
	SourceUserId string
	DestUserId   string
}
