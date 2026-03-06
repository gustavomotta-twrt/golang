package asana

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/TWRT/integration-mapper/internal/client"
	"github.com/TWRT/integration-mapper/internal/models"
)

type AsanaClient struct {
	baseUrl    string
	token      string
	httpClient *http.Client

	tagCacheMu sync.RWMutex
	tagCache   map[string]map[string]string // workspaceId → (tagName lowercase → GID)
}

func NewAsanaClient(token string) *AsanaClient {
	return &AsanaClient{
		baseUrl:    "https://app.asana.com/api/1.0",
		token:      token,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		tagCache:   make(map[string]map[string]string),
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

// parseAsanaTask converts an AsanaTasks (API type) into a models.Task (domain type).
func parseAsanaTask(asanaTask AsanaTasks) (models.Task, error) {
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
		return models.Task{}, err
	}

	var priority string
	for _, cf := range asanaTask.CustomFields {
		if cf.Name == "Priority" && cf.EnumValue != nil {
			priority = cf.EnumValue.Name
			break
		}
	}

	tags := make([]string, 0, len(asanaTask.Tags))
	for _, t := range asanaTask.Tags {
		tags = append(tags, t.Name)
	}

	return models.Task{
		Id:          asanaTask.Gid,
		Name:        asanaTask.Name,
		Description: asanaTask.Notes,
		Status:      status,
		Assignees:   assignees,
		DueDate:     dueDate,
		Priority:    priority,
		Tags:        tags,
	}, nil
}

func (c *AsanaClient) GetTasks(ctx context.Context, projectId string) ([]models.Task, error) {
	url := c.baseUrl + "/tasks?project=" + projectId +
		"&opt_fields=name,notes,completed,assignee,assignee.gid,assignee.name,assignee.email,due_on,custom_fields,custom_fields.name,custom_fields.enum_value,custom_fields.enum_value.name,tags,tags.name"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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
	for i, t := range asanaResp.Data {
		task, err := parseAsanaTask(t)
		if err != nil {
			return nil, err
		}
		tasks[i] = task
	}

	return tasks, nil
}

// CreateTask creates a task in Asana. The projectId param may be in the form
// "projectGid|sectionGid" to place the task inside a specific section.
func (c *AsanaClient) CreateTask(ctx context.Context, projectId string, workspaceId string, task models.Task) (*models.Task, error) {
	// Parse optional section from projectId
	actualProjectId := projectId
	sectionId := ""
	if idx := strings.Index(projectId, "|"); idx != -1 {
		actualProjectId = projectId[:idx]
		sectionId = projectId[idx+1:]
	}

	reqBody := CreateTaskRequest{
		Name:      task.Name,
		Notes:     task.Description,
		Completed: task.Status == "Completed",
		DueOn:     formatDueDate(task.DueDate),
	}

	reqBody.Projects = []string{actualProjectId}
	if sectionId != "" {
		reqBody.Memberships = []AsanaMembership{{Project: actualProjectId, Section: sectionId}}
	}

	if len(task.Assignees) > 0 {
		reqBody.Assignee = task.Assignees[0].ID
	}

	reqBody.CustomFields = make(map[string]interface{})

	if task.Priority != "" {
		parts := strings.SplitN(task.Priority, ":", 2)
		if len(parts) == 2 {
			reqBody.CustomFields[parts[0]] = parts[1]
		}
	}

	for _, cf := range task.CustomFields {
		if cf.Value != nil {
			reqBody.CustomFields[cf.FieldID] = cf.Value
		}
	}

	if len(reqBody.CustomFields) == 0 {
		reqBody.CustomFields = nil
	}

	if len(task.Tags) > 0 && workspaceId != "" {
		cachedTags, err := c.GetTagsForWorkspace(ctx, workspaceId)
		if err != nil {
			return nil, fmt.Errorf("get workspace tags for resolution (asana): %w", err)
		}

		tagGids := make([]string, 0, len(task.Tags))
		for _, tagName := range task.Tags {
			key := strings.ToLower(tagName)

			c.tagCacheMu.RLock()
			gid, ok := cachedTags[key]
			c.tagCacheMu.RUnlock()

			if !ok {
				newGid, err := c.CreateTag(ctx, workspaceId, tagName)
				if err != nil {
					return nil, fmt.Errorf("create tag %q (asana): %w", tagName, err)
				}
				gid = newGid

				c.tagCacheMu.Lock()
				cachedTags[key] = gid
				c.tagCacheMu.Unlock()
			}

			tagGids = append(tagGids, gid)
		}
		reqBody.Tags = tagGids
	}

	wrapper := CreateTaskRequestWrapper{Data: reqBody}

	body, err := json.Marshal(wrapper)
	if err != nil {
		return nil, fmt.Errorf("marshal create task request (asana): %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseUrl+"/tasks", bytes.NewBuffer(body))
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

func (c *AsanaClient) GetMembers(ctx context.Context, workspaceId string) ([]models.Member, error) {
	url := c.baseUrl + "/users?workspace=" + workspaceId + "&opt_fields=name,email"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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

func (c *AsanaClient) GetWorkspaces(ctx context.Context) ([]GetMultipleWorkspacesResponse, error) {
	url := c.baseUrl + "/workspaces"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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

func (c *AsanaClient) GetProjects(ctx context.Context, workspaceId string) ([]GetMultipleProjectsResponse, error) {
	url := c.baseUrl + "/projects?workspace=" + workspaceId

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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

func (c *AsanaClient) fetchTagsFromAPI(ctx context.Context, workspaceId string) (map[string]string, error) {
	tagMap := make(map[string]string)
	offset := ""

	for {
		url := c.baseUrl + "/tags?workspace=" + workspaceId + "&opt_fields=name&limit=100"
		if offset != "" {
			url += "&offset=" + offset
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("build request (asana tags): %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+c.token)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("get tags (asana): %w", err)
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read response body (asana tags): %w", readErr)
		}

		if resp.StatusCode != http.StatusOK {
			var asanaErr AsanaErrors
			if err := json.Unmarshal(body, &asanaErr); err != nil {
				return nil, fmt.Errorf("error status (asana tags): %d", resp.StatusCode)
			}
			if len(asanaErr.Errors) > 0 {
				return nil, fmt.Errorf("Asana error: %s", asanaErr.Errors[0].Message)
			}
			return nil, fmt.Errorf("API error status: %d", resp.StatusCode)
		}

		var result struct {
			Data     []AsanaTag `json:"data"`
			NextPage *struct {
				Offset string `json:"offset"`
			} `json:"next_page"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("parse tags (asana): %w", err)
		}

		for _, tag := range result.Data {
			tagMap[strings.ToLower(tag.Name)] = tag.Gid
		}

		if result.NextPage == nil || result.NextPage.Offset == "" {
			break
		}
		offset = result.NextPage.Offset
	}

	return tagMap, nil
}

func (c *AsanaClient) GetTagsForWorkspace(ctx context.Context, workspaceId string) (map[string]string, error) {
	// Fast path: cache hit
	c.tagCacheMu.RLock()
	if cached, ok := c.tagCache[workspaceId]; ok {
		c.tagCacheMu.RUnlock()
		return cached, nil
	}
	c.tagCacheMu.RUnlock()

	// Slow path: fetch from API without holding the lock
	tagMap, err := c.fetchTagsFromAPI(ctx, workspaceId)
	if err != nil {
		return nil, err
	}

	// Double-check: another goroutine may have populated the cache while we fetched
	c.tagCacheMu.Lock()
	if existing, ok := c.tagCache[workspaceId]; ok {
		c.tagCacheMu.Unlock()
		return existing, nil
	}
	c.tagCache[workspaceId] = tagMap
	c.tagCacheMu.Unlock()

	return tagMap, nil
}

func (c *AsanaClient) CreateTag(ctx context.Context, workspaceId, name string) (string, error) {
	wrapper := CreateTagRequestWrapper{
		Data: CreateTagRequest{
			Name:      name,
			Workspace: workspaceId,
		},
	}

	body, err := json.Marshal(wrapper)
	if err != nil {
		return "", fmt.Errorf("marshal create tag request (asana): %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseUrl+"/tags", bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("build request (asana create tag): %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("create tag (asana): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		errorBody, _ := io.ReadAll(resp.Body)
		var asanaErr AsanaErrors
		if err := json.Unmarshal(errorBody, &asanaErr); err != nil {
			return "", fmt.Errorf("error status (asana create tag): %d", resp.StatusCode)
		}
		if len(asanaErr.Errors) > 0 {
			return "", fmt.Errorf("Asana error: %s", asanaErr.Errors[0].Message)
		}
		return "", fmt.Errorf("API error status: %d", resp.StatusCode)
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response body (asana create tag): %w", err)
	}

	var result CreateTagResponse
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return "", fmt.Errorf("parse create tag response (asana): %w", err)
	}

	return result.Data.Gid, nil
}

// GetSourceContainers returns the sections of an Asana project (used as source containers).
func (c *AsanaClient) GetSourceContainers(ctx context.Context, projectId string) ([]client.Container, error) {
	sections, err := c.GetSections(ctx, projectId)
	if err != nil {
		return nil, err
	}
	containers := make([]client.Container, len(sections))
	for i, s := range sections {
		containers[i] = client.Container{ID: s.Gid, Name: s.Name}
	}
	return containers, nil
}

// GetTasksByContainer returns tasks in an Asana section.
func (c *AsanaClient) GetTasksByContainer(ctx context.Context, sectionId string) ([]models.Task, error) {
	return c.GetTasksBySection(ctx, sectionId)
}

// GetDestContainers returns the sections of an Asana project (used as destination containers).
func (c *AsanaClient) GetDestContainers(ctx context.Context, projectId string) ([]client.Container, error) {
	return c.GetSourceContainers(ctx, projectId)
}

func (c *AsanaClient) GetListStatuses(ctx context.Context, listId string) ([]string, error) {
	return []string{"Incomplete", "Completed"}, nil
}

func (c *AsanaClient) GetSections(ctx context.Context, projectId string) ([]AsanaSection, error) {
	url := c.baseUrl + "/projects/" + projectId + "/sections?opt_fields=name"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request (asana get sections): %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get sections (asana): %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body (asana get sections): %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var asanaErr AsanaErrors
		if err := json.Unmarshal(body, &asanaErr); err == nil && len(asanaErr.Errors) > 0 {
			return nil, fmt.Errorf("Asana error: %s", asanaErr.Errors[0].Message)
		}
		return nil, fmt.Errorf("API error status (asana get sections): %d", resp.StatusCode)
	}

	var result AsanaResponse[AsanaSection]
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse sections (asana): %w", err)
	}

	return result.Data, nil
}

// GetTasksBySection fetches all tasks belonging to a specific Asana section.
func (c *AsanaClient) GetTasksBySection(ctx context.Context, sectionId string) ([]models.Task, error) {
	url := c.baseUrl + "/tasks?section=" + sectionId +
		"&opt_fields=name,notes,completed,assignee,assignee.gid,assignee.name,assignee.email,due_on,custom_fields,custom_fields.name,custom_fields.enum_value,custom_fields.enum_value.name,tags,tags.name"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request (asana get tasks by section): %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get tasks by section (asana): %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body (asana get tasks by section): %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var asanaErr AsanaErrors
		if err := json.Unmarshal(body, &asanaErr); err == nil && len(asanaErr.Errors) > 0 {
			return nil, fmt.Errorf("Asana error: %s", asanaErr.Errors[0].Message)
		}
		return nil, fmt.Errorf("API error status (asana get tasks by section): %d", resp.StatusCode)
	}

	var asanaResp AsanaResponse[AsanaTasks]
	if err := json.Unmarshal(body, &asanaResp); err != nil {
		return nil, fmt.Errorf("parse tasks by section (asana): %w", err)
	}

	tasks := make([]models.Task, len(asanaResp.Data))
	for i, t := range asanaResp.Data {
		task, err := parseAsanaTask(t)
		if err != nil {
			return nil, err
		}
		tasks[i] = task
	}

	return tasks, nil
}

func (c *AsanaClient) GetProjectCustomFieldOptions(ctx context.Context, projectGid string) (map[string]string, error) {
	url := c.baseUrl + "/projects/" + projectGid + "/custom_field_settings" +
		"?opt_fields=custom_field.name,custom_field.gid,custom_field.enum_options,custom_field.enum_options.name,custom_field.enum_options.gid"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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

func (c *AsanaClient) CreateCustomField(ctx context.Context, workspaceId, name, asanaType string, options []string) (string, []string, error) {
	reqData := CreateCustomFieldRequest{
		Workspace: workspaceId,
		Name:      name,
		Type:      asanaType,
	}
	if len(options) > 0 && (asanaType == "enum" || asanaType == "multi_enum") {
		for _, opt := range options {
			reqData.EnumOptions = append(reqData.EnumOptions, AsanaEnumOptionInput{Name: opt})
		}
	}

	wrapper := CreateCustomFieldWrapper{Data: reqData}
	body, err := json.Marshal(wrapper)
	if err != nil {
		return "", nil, fmt.Errorf("marshal create custom field request (asana): %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseUrl+"/custom_fields", bytes.NewBuffer(body))
	if err != nil {
		return "", nil, fmt.Errorf("build request (asana create custom field): %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("create custom field (asana): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		errorBody, _ := io.ReadAll(resp.Body)
		var asanaErr AsanaErrors
		if err := json.Unmarshal(errorBody, &asanaErr); err != nil {
			return "", nil, fmt.Errorf("error status (asana create custom field): %d", resp.StatusCode)
		}
		if len(asanaErr.Errors) > 0 {
			return "", nil, fmt.Errorf("Asana error: %s", asanaErr.Errors[0].Message)
		}
		return "", nil, fmt.Errorf("API error status: %d", resp.StatusCode)
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("read response body (asana create custom field): %w", err)
	}

	var result CreateCustomFieldResponse
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return "", nil, fmt.Errorf("parse create custom field response (asana): %w", err)
	}

	optionGIDs := make([]string, 0, len(result.Data.EnumOptions))
	for _, opt := range result.Data.EnumOptions {
		optionGIDs = append(optionGIDs, opt.Gid)
	}

	return result.Data.Gid, optionGIDs, nil
}

func (c *AsanaClient) GetProjectCustomField(ctx context.Context, projectGid, name string) (string, []string, bool, error) {
	url := c.baseUrl + "/projects/" + projectGid + "/custom_field_settings" +
		"?opt_fields=custom_field.gid,custom_field.name,custom_field.enum_options,custom_field.enum_options.gid,custom_field.enum_options.name"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", nil, false, fmt.Errorf("build request (asana project custom fields): %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", nil, false, fmt.Errorf("get project custom fields (asana): %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, false, fmt.Errorf("read response (asana project custom fields): %w", err)
	}

	var result AsanaResponse[AsanaCustomFieldSetting]
	if err := json.Unmarshal(body, &result); err != nil {
		return "", nil, false, fmt.Errorf("parse project custom fields (asana): %w", err)
	}

	for _, setting := range result.Data {
		if setting.CustomField.Name == name {
			optionGIDs := make([]string, 0, len(setting.CustomField.EnumOptions))
			for _, opt := range setting.CustomField.EnumOptions {
				optionGIDs = append(optionGIDs, opt.Gid)
			}
			return setting.CustomField.Gid, optionGIDs, true, nil
		}
	}

	return "", nil, false, nil
}

func (c *AsanaClient) FindCustomFieldByName(ctx context.Context, workspaceId, name string) (string, []string, error) {
	baseURL := fmt.Sprintf("%s/workspaces/%s/custom_fields?opt_fields=gid,name,enum_options,enum_options.gid,enum_options.name&limit=100", c.baseUrl, workspaceId)
	nextURL := baseURL

	for nextURL != "" {
		req, err := http.NewRequestWithContext(ctx, "GET", nextURL, nil)
		if err != nil {
			return "", nil, fmt.Errorf("build request (asana find custom field): %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.token)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return "", nil, fmt.Errorf("find custom field (asana): %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return "", nil, fmt.Errorf("read response (asana find custom field): %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			var asanaErr AsanaErrors
			if jsonErr := json.Unmarshal(body, &asanaErr); jsonErr == nil && len(asanaErr.Errors) > 0 {
				return "", nil, fmt.Errorf("Asana error (list custom fields %d): %s", resp.StatusCode, asanaErr.Errors[0].Message)
			}
			return "", nil, fmt.Errorf("Asana error (list custom fields %d): %s", resp.StatusCode, string(body))
		}

		var result AsanaResponse[AsanaCreatedCustomField]
		if err := json.Unmarshal(body, &result); err != nil {
			return "", nil, fmt.Errorf("parse custom fields (asana find): %w", err)
		}

		for _, field := range result.Data {
			if field.Name == name {
				optionGIDs := make([]string, 0, len(field.EnumOptions))
				for _, opt := range field.EnumOptions {
					optionGIDs = append(optionGIDs, opt.Gid)
				}
				return field.Gid, optionGIDs, nil
			}
		}

		if result.NextPage != nil && result.NextPage.Offset != "" {
			nextURL = baseURL + "&offset=" + result.NextPage.Offset
		} else {
			nextURL = ""
		}
	}

	return "", nil, fmt.Errorf("custom field %q not found in workspace", name)
}

func (c *AsanaClient) AttachCustomFieldToProject(ctx context.Context, projectGid, fieldGid string) error {
	reqData := AddCustomFieldSettingRequest{
		Data: AddCustomFieldSettingData{CustomField: fieldGid},
	}

	body, err := json.Marshal(reqData)
	if err != nil {
		return fmt.Errorf("marshal attach custom field request (asana): %w", err)
	}

	url := c.baseUrl + "/projects/" + projectGid + "/addCustomFieldSetting"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("build request (asana attach custom field): %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("attach custom field to project (asana): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		errorBody, _ := io.ReadAll(resp.Body)
		var asanaErr AsanaErrors
		if err := json.Unmarshal(errorBody, &asanaErr); err != nil {
			return fmt.Errorf("error status (asana attach custom field): %d", resp.StatusCode)
		}
		if len(asanaErr.Errors) > 0 {
			msg := asanaErr.Errors[0].Message
			if strings.Contains(msg, "already present") || strings.Contains(msg, "already exists") {
				return nil
			}
			return fmt.Errorf("Asana error: %s", msg)
		}
		return fmt.Errorf("API error status: %d", resp.StatusCode)
	}

	return nil
}
