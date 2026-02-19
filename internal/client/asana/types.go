package asana

type AsanaResponse struct {
	Data []AsanaTasks
}

type AsanaTasks struct {
	Gid       string `json:"gid"`
	Name      string `json:"name"`
	Notes     string `json:"notes"`
	Completed bool   `json:"completed"`
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
}

type CreateTaskRequestWrapper struct {
	Data CreateTaskRequest `json:"data"`
}

type CreateTaskResponse struct {
	Data AsanaTasks `json:"data"`
}
