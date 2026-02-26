package clickup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/TWRT/integration-mapper/internal/models"
)

type ClickUpClient struct {
	baseUrl    string
	token      string
	httpClient *http.Client
}

func NewClickUpClient(token string) *ClickUpClient {
	return &ClickUpClient{
		baseUrl:    "https://api.clickup.com/api/v2",
		token:      token,
		httpClient: &http.Client{Timeout: 10 * time.Second},
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

func (c *ClickUpClient) GetTasks(listId string) ([]models.Task, error) {
	url := c.baseUrl + "/list/" + listId + "/task?include_closed=true"

	req, err := http.NewRequest("GET", url, nil)
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

		tasks[i] = models.Task{
			Id:          clickUpTask.Id,
			Name:        clickUpTask.Name,
			Description: clickUpTask.Description,
			Status:      clickUpTask.Status.Status,
			Assignees:   assignees,
			DueDate:     dueDate,
			Priority:    priority,
		}
	}

	return tasks, nil
}

func (c *ClickUpClient) CreateTask(listId string, task models.Task) (*models.Task, error) {
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
	}

	url := c.baseUrl + "/list/" + listId + "/task"

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal create task request (clickup): %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
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

func (c *ClickUpClient) GetWorkspaces() ([]ClickUpTeams, error) {
	url := c.baseUrl + "/team"

	req, err := http.NewRequest("GET", url, nil)
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

func (c *ClickUpClient) GetMembers(workspaceId string) ([]models.Member, error) {
	teams, err := c.GetWorkspaces()
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
			return members, nil
		}
	}
	return nil, fmt.Errorf("workspace %s not found (clickup)", workspaceId)
}

func (c *ClickUpClient) GetSpaces(workspaceId string) ([]ClickUpSpace, error) {
	url := c.baseUrl + "/team/" + workspaceId + "/space"

	req, err := http.NewRequest("GET", url, nil)
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

func (c *ClickUpClient) GetLists(spaceId string) ([]ClickUpList, error) {
	url := c.baseUrl + "/space/" + spaceId + "/list"

	req, err := http.NewRequest("GET", url, nil)
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

func (c *ClickUpClient) GetListStatuses(listId string) ([]string, error) {
	url := c.baseUrl + "/list/" + listId

	req, err := http.NewRequest("GET", url, nil)
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
