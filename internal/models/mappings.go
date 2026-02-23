package models

type StatusMapping struct {
	SourceStatus string `json:"source_status"`
	DestStatus   string `json:"dest_status"`
}

type AssigneeMapping struct {
	SourceUserId string `json:"source_user_id"`
	DestUserId   string `json:"dest_user_id"`
}
