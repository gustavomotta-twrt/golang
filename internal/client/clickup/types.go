package clickup

type ClickUpErrors struct {
	Err string `json:"err"`
	Code string `json:"ECODE"`
}

type ClickUpWorkspaces struct {
	Teams []ClickUpTeams
}

type ClickUpTeams struct {
	Id string `json:"id"`
	Name string `json:"name"`
	Color string `json:"color"`
	Avatar string  `json:"avatar"`
	Members []ClickupTeamsMembers
}

type ClickupTeamsMembers struct {
	Id string `json:"id"`
	Username string `json:"username"`
	Email string `json:"email"`
	Color string `json:"color"`
	ProfilePicture string `json:"profilePicture"`
	Initials string `json:"initials"`
	Role int `json:"role"`
	RoleSubtype int `json:"role_subtype"`
	RoleKey string `json:"role_key"`
	LastActive int64 `json:"last_active"`
	DateJoined int64 `json:"date_joined"`
	DataInvited int64 `json:"date_invited"`
}

type ClickUpTasks struct {
	Tasks []ClickUpTask `json:"tasks"`
}

type ClickUpStatus struct {
	Status string `json:"status"`
	Color string `json:"color"`
	OrderIndex int `json:"orderindex"`
	Type string `json:"type"`
}

type ClickUpCreator struct {
	Id int `json:"id"`
	Username string `json:"username"`
	Color string `json:"string"`
	ProfilePictureUrl string `json:"profilePicture"`
}

type ClickUpAssignees struct {
	Id int `json:"id"`
	Username string `json:"username"`
	Color string `json:"color"`
	Email string `json:"email"`
	ProfilePicture string `json:"profilePicture"`
}

type ClickUpPriority struct {
	Color string `json:"color"`
	Id string `json:"id"`
	OrderIndex string `json:"orderindex"`
	Priority string `json:"priority"`
}

type ClickUpTask struct {
	Id string `json:"id"`
	Name string `json:"name"`
	Status ClickUpStatus `json:"status"`
	OrderIndex string `json:"orderindex"`
	DateCreated string `json:"date_created"`
	DateUpdated string `json:"date_updated"`
	DateClosed *string `json:"date_closed"`
	Creator ClickUpCreator `json:"creator"`
	Assignees []ClickUpAssignees `json:"assignees"`
	Priority *ClickUpPriority `json:"priority"`
	DueDate *int64 `json:"due_date"`
	StartDate int64 `json:"start_date"`
	TimeEstimate int64 `json:"time_estimate"`
	TimeSpent int64 `json:"time_spent"`
}

type CreateTaskRequest struct {
	Name string `json:"name"`
	Description string `json:"description,omitempty"`
	Status string `json:"status,omitempty"`
}
