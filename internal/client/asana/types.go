package asana

type AsanaNextPage struct {
	Offset string `json:"offset"`
}

type AsanaResponse[T any] struct {
	Data     []T             `json:"data"`
	NextPage *AsanaNextPage  `json:"next_page"`
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

type AsanaTag struct {
	Gid  string `json:"gid"`
	Name string `json:"name"`
}

type AsanaTasks struct {
	Gid          string             `json:"gid"`
	Name         string             `json:"name"`
	Notes        string             `json:"notes"`
	Completed    bool               `json:"completed"`
	Assignee     *AsanaUser         `json:"assignee"`
	DueOn        string             `json:"due_on"`
	CustomFields []AsanaCustomField `json:"custom_fields"`
	Tags         []AsanaTag         `json:"tags"`
}

type AsanaDetailError struct {
	Message string `json:"message"`
	Help    string `json:"help"`
}

type AsanaErrors struct {
	Errors []AsanaDetailError `json:"errors"`
}

type CreateTaskRequest struct {
	Name         string                 `json:"name"`
	Notes        string                 `json:"notes,omitempty"`
	Projects     []string               `json:"projects,omitempty"`
	Memberships  []AsanaMembership      `json:"memberships,omitempty"`
	Completed    bool                   `json:"completed"`
	Assignee     string                 `json:"assignee,omitempty"`
	DueOn        string                 `json:"due_on,omitempty"`
	CustomFields map[string]interface{} `json:"custom_fields,omitempty"`
	Tags         []string               `json:"tags,omitempty"`
}

type AsanaEnumOptionInput struct {
	Name string `json:"name"`
}

type CreateCustomFieldRequest struct {
	Workspace   string                 `json:"workspace"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	EnumOptions []AsanaEnumOptionInput `json:"enum_options,omitempty"`
}

type CreateCustomFieldWrapper struct {
	Data CreateCustomFieldRequest `json:"data"`
}

type AsanaCreatedEnumOption struct {
	Gid  string `json:"gid"`
	Name string `json:"name"`
}

type AsanaCreatedCustomField struct {
	Gid         string                   `json:"gid"`
	Name        string                   `json:"name"`
	EnumOptions []AsanaCreatedEnumOption `json:"enum_options"`
}

type CreateCustomFieldResponse struct {
	Data AsanaCreatedCustomField `json:"data"`
}

type AddCustomFieldSettingData struct {
	CustomField string `json:"custom_field"`
}

type AddCustomFieldSettingRequest struct {
	Data AddCustomFieldSettingData `json:"data"`
}

type CreateTagRequest struct {
	Name      string `json:"name"`
	Workspace string `json:"workspace"`
}

type CreateTagRequestWrapper struct {
	Data CreateTagRequest `json:"data"`
}

type CreateTagResponse struct {
	Data AsanaTag `json:"data"`
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

type AsanaSection struct {
	Gid  string `json:"gid"`
	Name string `json:"name"`
}

type AsanaMembership struct {
	Project string `json:"project"`
	Section string `json:"section"`
}
