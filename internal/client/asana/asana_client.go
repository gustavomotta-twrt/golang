package asana

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/TWRT/integration-mapper/internal/models"
)

type AsanaClient struct {
	baseUrl    string
	token      string
	httpClient *http.Client
}

func NewAsanaClient(token string) *AsanaClient {
	return &AsanaClient{
		baseUrl:    "https://app.asana.com/api/1.0",
		token:      token,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func parseDueDate(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, fmt.Errorf("parse due_date (asana): %w", err)
	}
	utc := time.Date(t.Year(), t.Month(), t.Day(), 12, 0, 0, 0, time.UTC)
	return &utc, nil
}

func formatDueDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

func (c *AsanaClient) GetTasks(projectId string) ([]models.Task, error) {
	url := c.baseUrl + "/tasks?project=" + projectId +
		"&opt_fields=name,notes,completed,assignee,assignee.gid,assignee.name,assignee.email,due_on,custom_fields,custom_fields.name,custom_fields.enum_value,custom_fields.enum_value.name"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request (asana): %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get tasks (asana): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read error body (asana): %w", err)
		}

		var asanaErr AsanaErrors
		if err := json.Unmarshal(errorBody, &asanaErr); err != nil {
			return nil, fmt.Errorf("error status (asana): %d", resp.StatusCode)
		}
		if len(asanaErr.Errors) > 0 {
			return nil, fmt.Errorf("Asana error: %s", asanaErr.Errors[0].Message)
		}
		return nil, fmt.Errorf("API error status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body (asana): %w", err)
	}

	var asanaResp AsanaResponse[AsanaTasks]
	if err := json.Unmarshal(body, &asanaResp); err != nil {
		return nil, fmt.Errorf("parse tasks (asana): %w", err)
	}

	tasks := make([]models.Task, len(asanaResp.Data))
	for i, asanaTask := range asanaResp.Data {
		status := "Incomplete"
		if asanaTask.Completed {
			status = "Completed"
		}

		var assignees []models.TaskAssignee
		if asanaTask.Assignee != nil {
			assignees = []models.TaskAssignee{{
				ID:    asanaTask.Assignee.Gid,
				Name:  asanaTask.Assignee.Name,
				Email: asanaTask.Assignee.Email,
			}}
		}

		dueDate, err := parseDueDate(asanaTask.DueOn)
		if err != nil {
			return nil, err
		}

		var priority string
		for _, cf := range asanaTask.CustomFields {
			if cf.Name == "Priority" && cf.EnumValue != nil {
				priority = cf.EnumValue.Name
				break
			}
		}

		tasks[i] = models.Task{
			Id:          asanaTask.Gid,
			Name:        asanaTask.Name,
			Description: asanaTask.Notes,
			Status:      status,
			Assignees:   assignees,
			DueDate:     dueDate,
			Priority:    priority,
		}
	}

	return tasks, nil
}

func (c *AsanaClient) CreateTask(projectId string, task models.Task) (*models.Task, error) {
	reqBody := CreateTaskRequest{
		Name:      task.Name,
		Notes:     task.Description,
		Completed: task.Status == "Completed",
		Projects:  []string{projectId},
		DueOn:     formatDueDate(task.DueDate),
	}

	if len(task.Assignees) > 0 {
		reqBody.Assignee = task.Assignees[0].ID
	}

	if task.Priority != "" {
		parts := strings.SplitN(task.Priority, ":", 2)
		if len(parts) == 2 {
			reqBody.CustomFields = map[string]string{
				parts[0]: parts[1],
			}
		}
	}

	wrapper := CreateTaskRequestWrapper{Data: reqBody}

	body, err := json.Marshal(wrapper)
	if err != nil {
		return nil, fmt.Errorf("marshal create task request (asana): %w", err)
	}

	req, err := http.NewRequest("POST", c.baseUrl+"/tasks", bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("build request (asana): %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create task (asana): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		errorBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read error body (asana): %w", err)
		}
		var asanaErr AsanaErrors
		if err := json.Unmarshal(errorBody, &asanaErr); err != nil {
			return nil, fmt.Errorf("error status (asana): %d", resp.StatusCode)
		}
		if len(asanaErr.Errors) > 0 {
			return nil, fmt.Errorf("Asana error: %s", asanaErr.Errors[0].Message)
		}
		return nil, fmt.Errorf("API error status: %d", resp.StatusCode)
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body (asana): %w", err)
	}

	var createdTaskResp CreateTaskResponse
	if err := json.Unmarshal(responseBody, &createdTaskResp); err != nil {
		return nil, fmt.Errorf("parse create task response (asana): %w", err)
	}

	status := "Incomplete"
	if createdTaskResp.Data.Completed {
		status = "Completed"
	}

	return &models.Task{
		Id:          createdTaskResp.Data.Gid,
		Name:        createdTaskResp.Data.Name,
		Description: createdTaskResp.Data.Notes,
		Status:      status,
		Completed:   createdTaskResp.Data.Completed,
	}, nil
}

func (c *AsanaClient) GetMembers(workspaceId string) ([]models.Member, error) {
	url := c.baseUrl + "/users?workspace=" + workspaceId + "&opt_fields=name,email"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request (asana): %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get members (asana): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, _ := io.ReadAll(resp.Body)
		var asanaErr AsanaErrors
		if err := json.Unmarshal(errorBody, &asanaErr); err != nil {
			return nil, fmt.Errorf("get members (asana): status %d", resp.StatusCode)
		}
		if len(asanaErr.Errors) > 0 {
			return nil, fmt.Errorf("Asana error: %s", asanaErr.Errors[0].Message)
		}
		return nil, fmt.Errorf("get members (asana): status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response (asana): %w", err)
	}

	var asanaResp AsanaResponse[AsanaUser]
	if err := json.Unmarshal(body, &asanaResp); err != nil {
		return nil, fmt.Errorf("parse users (asana): %w", err)
	}

	members := make([]models.Member, 0, len(asanaResp.Data))
	for _, u := range asanaResp.Data {
		members = append(members, models.Member{
			ID:    u.Gid,
			Name:  u.Name,
			Email: u.Email,
		})
	}
	return members, nil
}

func (c *AsanaClient) GetWorkspaces() ([]GetMultipleWorkspacesResponse, error) {
	url := c.baseUrl + "/workspaces"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request (asana): %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get workspaces (asana): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read error body (asana): %w", err)
		}

		var asanaErr AsanaErrors
		if err := json.Unmarshal(errorBody, &asanaErr); err != nil {
			return nil, fmt.Errorf("error status (asana): %d", resp.StatusCode)
		}
		if len(asanaErr.Errors) > 0 {
			return nil, fmt.Errorf("Asana error: %s", asanaErr.Errors[0].Message)
		}
		return nil, fmt.Errorf("API error status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body (asana): %w", err)
	}

	var asanaResp AsanaResponse[GetMultipleWorkspacesResponse]
	if err := json.Unmarshal(body, &asanaResp); err != nil {
		return nil, fmt.Errorf("parse workspaces (asana): %w", err)
	}

	return asanaResp.Data, nil
}

func (c *AsanaClient) GetProjects(workspaceId string) ([]GetMultipleProjectsResponse, error) {
	url := c.baseUrl + "/projects?workspace=" + workspaceId

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request (asana): %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get projects (asana): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read error body (asana): %w", err)
		}

		var asanaErr AsanaErrors
		if err := json.Unmarshal(errorBody, &asanaErr); err != nil {
			return nil, fmt.Errorf("error status (asana): %d", resp.StatusCode)
		}
		if len(asanaErr.Errors) > 0 {
			return nil, fmt.Errorf("Asana error: %s", asanaErr.Errors[0].Message)
		}
		return nil, fmt.Errorf("API error status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body (asana): %w", err)
	}

	var asanaResp AsanaResponse[GetMultipleProjectsResponse]
	if err := json.Unmarshal(body, &asanaResp); err != nil {
		return nil, fmt.Errorf("parse projects (asana): %w", err)
	}

	return asanaResp.Data, nil
}

// TODO: Asana suporta status customizados via seções do projeto — implementar futuramente.
func (c *AsanaClient) GetListStatuses(listId string) ([]string, error) {
	return []string{"Incomplete", "Completed"}, nil
}

func (c *AsanaClient) GetProjectCustomFieldOptions(projectGid string) (map[string]string, error) {
	url := c.baseUrl + "/projects/" + projectGid + "/custom_field_settings" +
		"?opt_fields=custom_field.name,custom_field.gid,custom_field.enum_options,custom_field.enum_options.name,custom_field.enum_options.gid"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request (asana): %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get project custom field settings (asana): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, _ := io.ReadAll(resp.Body)
		var asanaErr AsanaErrors
		if err := json.Unmarshal(errorBody, &asanaErr); err != nil {
			return nil, fmt.Errorf("error status (asana): %d", resp.StatusCode)
		}
		if len(asanaErr.Errors) > 0 {
			return nil, fmt.Errorf("Asana error: %s", asanaErr.Errors[0].Message)
		}
		return nil, fmt.Errorf("API error status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body (asana): %w", err)
	}

	var settingsResp AsanaResponse[AsanaCustomFieldSetting]
	if err := json.Unmarshal(body, &settingsResp); err != nil {
		return nil, fmt.Errorf("parse custom field settings (asana): %w", err)
	}

	for _, s := range settingsResp.Data {
		if s.CustomField.Name == "Priority" {
			cf := s.CustomField
			optionMap := make(map[string]string, len(cf.EnumOptions)+1)
			optionMap["__field_gid__"] = cf.Gid
			for _, opt := range cf.EnumOptions {
				optionMap[opt.Name] = opt.Gid
			}
			return optionMap, nil
		}
	}

	return map[string]string{}, nil
}
