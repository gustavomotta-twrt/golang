package asana

type AsanaResponse[T any] struct {
	Data []T `json:"data"`
}

type AsanaSingleResponse[T any] struct {
	Data T `json:"data"`
}

type AsanaUser struct {
	Gid   string `json:"gid"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type AsanaCustomFieldEnumValue struct {
	Gid  string `json:"gid"`
	Name string `json:"name"`
}

type AsanaCustomField struct {
	Gid       string                     `json:"gid"`
	Name      string                     `json:"name"`
	EnumValue *AsanaCustomFieldEnumValue `json:"enum_value"`
}

type AsanaEnumOption struct {
	Gid  string `json:"gid"`
	Name string `json:"name"`
}

type AsanaProjectCustomField struct {
	Gid         string            `json:"gid"`
	Name        string            `json:"name"`
	EnumOptions []AsanaEnumOption `json:"enum_options"`
}

type AsanaProject struct {
	Gid          string                    `json:"gid"`
	CustomFields []AsanaProjectCustomField `json:"custom_fields"`
}

type AsanaCustomFieldSetting struct {
	Gid         string                  `json:"gid"`
	CustomField AsanaProjectCustomField `json:"custom_field"`
}

type AsanaTasks struct {
	Gid          string             `json:"gid"`
	Name         string             `json:"name"`
	Notes        string             `json:"notes"`
	Completed    bool               `json:"completed"`
	Assignee     *AsanaUser         `json:"assignee"`
	DueOn        string             `json:"due_on"`
	CustomFields []AsanaCustomField `json:"custom_fields"`
}

type AsanaDetailError struct {
	Message string `json:"message"`
	Help    string `json:"help"`
}

type AsanaErrors struct {
	Errors []AsanaDetailError `json:"errors"`
}

type CreateTaskRequest struct {
	Name         string            `json:"name"`
	Notes        string            `json:"notes,omitempty"`
	Projects     []string          `json:"projects,omitempty"`
	Completed    bool              `json:"completed"`
	Assignee     string            `json:"assignee,omitempty"`
	DueOn        string            `json:"due_on,omitempty"`
	CustomFields map[string]string `json:"custom_fields,omitempty"`
}

type CreateTaskRequestWrapper struct {
	Data CreateTaskRequest `json:"data"`
}

type CreateTaskResponse struct {
	Data AsanaTasks `json:"data"`
}

type GetMultipleWorkspacesResponse struct {
	Id           string `json:"gid"`
	ResourceType string `json:"resource_type"`
	Name         string `json:"name"`
}

type GetMultipleProjectsResponse struct {
	Id           string `json:"gid"`
	ResourceType string `json:"resource_type"`
	Name         string `json:"name"`
}

type AsanaPriorityFieldInfo struct {
	FieldGid  string
	OptionMap map[string]string
}
