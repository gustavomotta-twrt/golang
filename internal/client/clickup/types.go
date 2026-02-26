package clickup

type ClickUpErrors struct {
	Err  string `json:"err"`
	Code string `json:"ECODE"`
}

type GetMultipleWorkspacesResponse struct {
	Teams []ClickUpTeams `json:"teams"`
}

type GetMultipleSpacesResponse struct {
	Spaces []ClickUpSpace `json:"spaces"`
}

type ClickUpSpace struct {
	Id     string `json:"id"`
	Name   string `json:"name"`
	Color  string `json:"color"`
	Avatar string `json:"avatar"`
}

type ClickupTeamMember struct {
	User ClickupTeamMemberUser `json:"user"`
}

type ClickupTeamMemberUser struct {
	Id             int    `json:"id"`
	Username       string `json:"username"`
	Email          string `json:"email"`
	Color          string `json:"color"`
	ProfilePicture string `json:"profilePicture"`
	Initials       string `json:"initials"`
	Role           int    `json:"role"`
	RoleSubtype    int    `json:"role_subtype"`
	RoleKey        string `json:"role_key"`
	LastActive     string `json:"last_active"`
	DateJoined     string `json:"date_joined"`
	DateInvited    string `json:"date_invited"`
}

type ClickUpTeams struct {
	Id      string              `json:"id"`
	Name    string              `json:"name"`
	Color   string              `json:"color"`
	Avatar  string              `json:"avatar"`
	Members []ClickupTeamMember `json:"members"`
}

type ClickUpTasks struct {
	Tasks []ClickUpTask `json:"tasks"`
}

type ClickUpStatus struct {
	Status     string `json:"status"`
	Color      string `json:"color"`
	OrderIndex int    `json:"orderindex"`
	Type       string `json:"type"`
}

type ClickUpCreator struct {
	Id                int    `json:"id"`
	Username          string `json:"username"`
	Color             string `json:"string"`
	ProfilePictureUrl string `json:"profilePicture"`
}

type ClickUpAssignees struct {
	Id             int    `json:"id"`
	Username       string `json:"username"`
	Color          string `json:"color"`
	Email          string `json:"email"`
	ProfilePicture string `json:"profilePicture"`
}

type ClickUpPriority struct {
	Color      string `json:"color"`
	Id         string `json:"id"`
	OrderIndex string `json:"orderindex"`
	Priority   string `json:"priority"`
}

type ClickUpTag struct {
	Name string `json:"name"`
}

type ClickUpTask struct {
	Id           string             `json:"id"`
	Name         string             `json:"name"`
	Description  string             `json:"description"`
	Status       ClickUpStatus      `json:"status"`
	OrderIndex   string             `json:"orderindex"`
	DateCreated  string             `json:"date_created"`
	DateUpdated  string             `json:"date_updated"`
	DateClosed   *string            `json:"date_closed"`
	Creator      ClickUpCreator     `json:"creator"`
	Assignees    []ClickUpAssignees `json:"assignees"`
	Priority     *ClickUpPriority   `json:"priority"`
	DueDate      string             `json:"due_date"`
	StartDate    int64              `json:"start_date"`
	TimeEstimate int64              `json:"time_estimate"`
	TimeSpent    int64              `json:"time_spent"`
	Tags         []ClickUpTag       `json:"tags"`
}

type CreateTaskRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Status      string   `json:"status,omitempty"`
	Assignees   []int    `json:"assignees,omitempty"`
	DueDate     *int64   `json:"due_date,omitempty"`
	Priority    *int     `json:"priority,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

type ClickUpListStatus struct {
	Status     string `json:"status"`
	Color      string `json:"color"`
	OrderIndex int    `json:"orderindex"`
	Type       string `json:"type"`
}

type ClickUpList struct {
	Id       string              `json:"id"`
	Name     string              `json:"name"`
	Statuses []ClickUpListStatus `json:"statuses"`
}

type GetMultipleListsResponse struct {
	Lists []ClickUpList `json:"lists"`
}
