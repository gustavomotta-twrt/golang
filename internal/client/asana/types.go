package asana

type AsanaResponse[T any] struct {
	Data []T `json:"data"`
}

type AsanaUser struct {
	Gid   string `json:"gid"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type AsanaTasks struct {
	Gid       string     `json:"gid"`
	Name      string     `json:"name"`
	Notes     string     `json:"notes"`
	Completed bool       `json:"completed"`
	Assignee  *AsanaUser `json:"assignee"`
}

type AsanaDetailError struct {
	Message string `json:"message"`
	Help    string `json:"help"`
}

type AsanaErrors struct {
	Errors []AsanaDetailError `json:"errors"`
}

type CreateTaskRequest struct {
	Name      string   `json:"name"`
	Notes     string   `json:"notes,omitempty"`
	Projects  []string `json:"projects,omitempty"`
	Completed bool     `json:"completed"`
	Assignee  string   `json:"assignee,omitempty"`
}

type CreateTaskRequestWrapper struct {
	Data CreateTaskRequest `json:"data"`
}

type CreateTaskResponse struct {
	Data AsanaTasks `json:"data"`
}

type GetMultipleWorkspacesResponse struct {
	Id          string `json:"gid"`
	ResourceType string `json:"resource_type"`
	Name         string `json:"name"`
}

type GetMultipleProjectsResponse struct {
	Id           string `json:"gid"`
	ResourceType string `json:"resource_type"`
	Name         string `json:"name"`
}
