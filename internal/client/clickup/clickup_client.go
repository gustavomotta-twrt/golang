package clickup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TWRT/integration-mapper/internal/client"
	"github.com/TWRT/integration-mapper/internal/models"
)

var clickUpSystemFieldPrefixes = []string{"BASELINE_"}

func isSystemField(name string) bool {
	for _, prefix := range clickUpSystemFieldPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

type ClickUpClient struct {
	baseUrl    string
	token      string
	httpClient *http.Client

	memberCacheMu sync.RWMutex
	memberCache   map[string][]models.Member // workspaceId → members
}

func NewClickUpClient(token string) *ClickUpClient {
	return &ClickUpClient{
		baseUrl:     "https://api.clickup.com/api/v2",
		token:       token,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		memberCache: make(map[string][]models.Member),
	}
}

func parseClickUpDueDate(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	ms, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse due_date (clickup): %w", err)
	}
	t := time.UnixMilli(ms).UTC()
	return &t, nil
}

func timeToMs(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	ms := t.UnixMilli()
	return &ms
}

func priorityStringToInt(p string) *int {
	m := map[string]int{
		"urgent": 1,
		"high":   2,
		"normal": 3,
		"low":    4,
	}
	if v, ok := m[p]; ok {
		return &v
	}
	return nil
}

func (c *ClickUpClient) GetTasks(ctx context.Context, listId string) ([]models.Task, error) {
	url := c.baseUrl + "/list/" + listId + "/task?include_closed=true"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request (clickup): %w", err)
	}

	req.Header.Set("Authorization", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get tasks (clickup): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read error body (clickup): %w", err)
		}

		var clickupErr ClickUpErrors
		if err := json.Unmarshal(errorBody, &clickupErr); err != nil {
			return nil, fmt.Errorf("error status (clickup): %d", resp.StatusCode)
		}
		if len(clickupErr.Err) > 0 {
			return nil, fmt.Errorf("ClickUp error: %s", clickupErr.Err)
		}
		return nil, fmt.Errorf("API error status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body (clickup): %w", err)
	}

	var clickUpResp ClickUpTasks
	if err := json.Unmarshal(body, &clickUpResp); err != nil {
		return nil, fmt.Errorf("parse tasks (clickup): %w", err)
	}

	tasks := make([]models.Task, len(clickUpResp.Tasks))
	for i, clickUpTask := range clickUpResp.Tasks {
		assignees := make([]models.TaskAssignee, 0, len(clickUpTask.Assignees))
		for _, a := range clickUpTask.Assignees {
			assignees = append(assignees, models.TaskAssignee{
				ID:    fmt.Sprintf("%d", a.Id),
				Name:  a.Username,
				Email: a.Email,
			})
		}

		dueDate, err := parseClickUpDueDate(clickUpTask.DueDate)
		if err != nil {
			return nil, err
		}

		var priority string
		if clickUpTask.Priority != nil {
			priority = clickUpTask.Priority.Priority
		}

		tags := make([]string, 0, len(clickUpTask.Tags))
		for _, t := range clickUpTask.Tags {
			tags = append(tags, t.Name)
		}

		customFields := make([]models.TaskCustomField, 0, len(clickUpTask.CustomFields))
		for _, cf := range clickUpTask.CustomFields {
			if len(cf.Value) == 0 || string(cf.Value) == "null" {
				continue
			}
			var rawValue interface{}
			if err := json.Unmarshal(cf.Value, &rawValue); err != nil {
				continue
			}
			customFields = append(customFields, models.TaskCustomField{
				FieldID: cf.ID,
				Value:   rawValue,
			})
		}

		tasks[i] = models.Task{
			Id:           clickUpTask.Id,
			Name:         clickUpTask.Name,
			Description:  clickUpTask.Description,
			Status:       clickUpTask.Status.Status,
			Assignees:    assignees,
			DueDate:      dueDate,
			Priority:     priority,
			Tags:         tags,
			CustomFields: customFields,
		}
	}

	return tasks, nil
}

func (c *ClickUpClient) GetFieldDefinitions(ctx context.Context, listId string) ([]models.CustomFieldDefinition, error) {
	fields, err := c.GetListCustomFields(ctx, listId)
	if err != nil {
		return nil, fmt.Errorf("get field definitions (clickup): %w", err)
	}

	defs := make([]models.CustomFieldDefinition, 0, len(fields))
	for _, f := range fields {
		if isSystemField(f.Name) {
			continue
		}
		opts := make([]models.CustomFieldOption, 0, len(f.TypeConfig.Options))
		for _, o := range f.TypeConfig.Options {
			name := o.Name
			if name == "" {
				name = o.Label
			}
			opts = append(opts, models.CustomFieldOption{
				ID:         o.Id,
				Name:       name,
				OrderIndex: o.OrderIndex,
			})
		}
		defs = append(defs, models.CustomFieldDefinition{
			ID:          f.Id,
			Name:        f.Name,
			ClickUpType: f.Type,
			Options:     opts,
		})
	}
	return defs, nil
}

func (c *ClickUpClient) CreateTask(ctx context.Context, listId string, _ string, task models.Task) (*models.Task, error) {
	assignees := make([]int, 0, len(task.Assignees))
	for _, a := range task.Assignees {
		id, err := strconv.Atoi(a.ID)
		if err != nil {
			continue
		}
		assignees = append(assignees, id)
	}

	reqBody := CreateTaskRequest{
		Name:        task.Name,
		Description: task.Description,
		Status:      task.Status,
		Assignees:   assignees,
		DueDate:     timeToMs(task.DueDate),
		Priority:    priorityStringToInt(task.Priority),
		Tags:        task.Tags,
	}

	url := c.baseUrl + "/list/" + listId + "/task"

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal create task request (clickup): %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("build request (clickup): %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create task (clickup): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read error body (clickup): %w", err)
		}

		var clickupErr ClickUpErrors
		if err := json.Unmarshal(errorBody, &clickupErr); err != nil {
			return nil, fmt.Errorf("error status (clickup): %d", resp.StatusCode)
		}
		if len(clickupErr.Err) > 0 {
			return nil, fmt.Errorf("ClickUp error: %s", clickupErr.Err)
		}
		return nil, fmt.Errorf("API error status: %d", resp.StatusCode)
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body (clickup): %w", err)
	}

	var createdTask ClickUpTask
	if err := json.Unmarshal(responseBody, &createdTask); err != nil {
		return nil, fmt.Errorf("parse create task response (clickup): %w", err)
	}

	return &models.Task{
		Id:     createdTask.Id,
		Name:   createdTask.Name,
		Status: createdTask.Status.Status,
	}, nil
}

func (c *ClickUpClient) GetWorkspaces(ctx context.Context) ([]ClickUpTeams, error) {
	url := c.baseUrl + "/team"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request (clickup): %w", err)
	}

	req.Header.Set("Authorization", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get workspaces (clickup): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read error body (clickup): %w", err)
		}

		var clickupErr ClickUpErrors
		if err := json.Unmarshal(errorBody, &clickupErr); err != nil {
			return nil, fmt.Errorf("error status (clickup): %d", resp.StatusCode)
		}
		if len(clickupErr.Err) > 0 {
			return nil, fmt.Errorf("ClickUp error: %s", clickupErr.Err)
		}
		return nil, fmt.Errorf("API error status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body (clickup): %w", err)
	}

	var clickupResp GetMultipleWorkspacesResponse
	if err := json.Unmarshal(body, &clickupResp); err != nil {
		return nil, fmt.Errorf("parse workspaces (clickup): %w", err)
	}

	return clickupResp.Teams, nil
}

func (c *ClickUpClient) GetMembers(ctx context.Context, workspaceId string) ([]models.Member, error) {
	// Fast path: cache hit
	c.memberCacheMu.RLock()
	if cached, ok := c.memberCache[workspaceId]; ok {
		c.memberCacheMu.RUnlock()
		return cached, nil
	}
	c.memberCacheMu.RUnlock()

	// Slow path: fetch from API
	teams, err := c.GetWorkspaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("get workspace members (clickup): %w", err)
	}

	for _, team := range teams {
		if team.Id == workspaceId {
			members := make([]models.Member, 0, len(team.Members))
			for _, m := range team.Members {
				members = append(members, models.Member{
					ID:    fmt.Sprintf("%d", m.User.Id),
					Name:  m.User.Username,
					Email: m.User.Email,
				})
			}

			// Double-check: another goroutine may have populated while we fetched
			c.memberCacheMu.Lock()
			if existing, ok := c.memberCache[workspaceId]; ok {
				c.memberCacheMu.Unlock()
				return existing, nil
			}
			c.memberCache[workspaceId] = members
			c.memberCacheMu.Unlock()

			return members, nil
		}
	}
	return nil, fmt.Errorf("workspace %s not found (clickup)", workspaceId)
}

func (c *ClickUpClient) GetSpaces(ctx context.Context, workspaceId string) ([]ClickUpSpace, error) {
	url := c.baseUrl + "/team/" + workspaceId + "/space"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request (clickup): %w", err)
	}

	req.Header.Set("Authorization", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get spaces (clickup): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read error body (clickup): %w", err)
		}

		var clickupErr ClickUpErrors
		if err := json.Unmarshal(errorBody, &clickupErr); err != nil {
			return nil, fmt.Errorf("parse error response (clickup): %w", err)
		}
		if clickupErr.Err != "" {
			return nil, fmt.Errorf("ClickUp error: %s", clickupErr.Err)
		}
		return nil, fmt.Errorf("API error status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body (clickup): %w", err)
	}

	var clickupResp GetMultipleSpacesResponse
	if err := json.Unmarshal(body, &clickupResp); err != nil {
		return nil, fmt.Errorf("parse spaces response (clickup): %w", err)
	}

	return clickupResp.Spaces, nil
}

func (c *ClickUpClient) GetLists(ctx context.Context, spaceId string) ([]ClickUpList, error) {
	url := c.baseUrl + "/space/" + spaceId + "/list"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request (clickup): %w", err)
	}

	req.Header.Set("Authorization", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get lists (clickup): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read error body (clickup): %w", err)
		}

		var clickupErr ClickUpErrors
		if err := json.Unmarshal(errorBody, &clickupErr); err != nil {
			return nil, fmt.Errorf("parse error response (clickup): %w", err)
		}
		if clickupErr.Err != "" {
			return nil, fmt.Errorf("ClickUp error: %s", clickupErr.Err)
		}
		return nil, fmt.Errorf("API error status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body (clickup): %w", err)
	}

	var clickupResp GetMultipleListsResponse
	if err := json.Unmarshal(body, &clickupResp); err != nil {
		return nil, fmt.Errorf("parse lists response (clickup): %w", err)
	}

	return clickupResp.Lists, nil
}

func (c *ClickUpClient) GetListCustomFields(ctx context.Context, listId string) ([]ClickUpCustomField, error) {
	url := c.baseUrl + "/list/" + listId + "/field"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request (clickup): %w", err)
	}

	req.Header.Set("Authorization", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get list custom fields (clickup): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read error body (clickup): %w", err)
		}

		var clickupErr ClickUpErrors
		if err := json.Unmarshal(errorBody, &clickupErr); err != nil {
			return nil, fmt.Errorf("error status (clickup): %d", resp.StatusCode)
		}
		if clickupErr.Err != "" {
			return nil, fmt.Errorf("ClickUp error: %s", clickupErr.Err)
		}
		return nil, fmt.Errorf("API error status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body (clickup): %w", err)
	}

	var clickupResp GetListCustomFieldsResponse
	if err := json.Unmarshal(body, &clickupResp); err != nil {
		return nil, fmt.Errorf("parse custom fields (clickup): %w", err)
	}

	return clickupResp.Fields, nil
}

// GetSourceContainers returns the lists of a ClickUp space (used as source containers).
func (c *ClickUpClient) GetSourceContainers(ctx context.Context, spaceId string) ([]client.Container, error) {
	lists, err := c.GetLists(ctx, spaceId)
	if err != nil {
		return nil, err
	}
	containers := make([]client.Container, len(lists))
	for i, l := range lists {
		containers[i] = client.Container{ID: l.Id, Name: l.Name}
	}
	return containers, nil
}

// GetTasksByContainer returns tasks in a ClickUp list.
func (c *ClickUpClient) GetTasksByContainer(ctx context.Context, listId string) ([]models.Task, error) {
	return c.GetTasks(ctx, listId)
}

// GetDestContainers returns the lists of a ClickUp space (used as destination containers).
func (c *ClickUpClient) GetDestContainers(ctx context.Context, spaceId string) ([]client.Container, error) {
	return c.GetSourceContainers(ctx, spaceId)
}

func (c *ClickUpClient) GetListStatuses(ctx context.Context, listId string) ([]string, error) {
	url := c.baseUrl + "/list/" + listId

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request (clickup): %w", err)
	}

	req.Header.Set("Authorization", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get list (clickup): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read error body (clickup): %w", err)
		}

		var clickupErr ClickUpErrors
		if err := json.Unmarshal(errorBody, &clickupErr); err != nil {
			return nil, fmt.Errorf("error status (clickup): %d", resp.StatusCode)
		}
		if clickupErr.Err != "" {
			return nil, fmt.Errorf("ClickUp error: %s", clickupErr.Err)
		}
		return nil, fmt.Errorf("API error status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body (clickup): %w", err)
	}

	var list ClickUpList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("parse list (clickup): %w", err)
	}

	statuses := make([]string, 0, len(list.Statuses))
	for _, s := range list.Statuses {
		statuses = append(statuses, s.Status)
	}
	return statuses, nil
}
