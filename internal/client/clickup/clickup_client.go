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

func (c *ClickUpClient) GetTasks(listId string) ([]models.Task, error) {
	url := c.baseUrl + "/list/" + listId + "/task?include_closed=true"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("Error trying to read the body: %w", err)
		}

		var clickupErr ClickUpErrors

		if err := json.Unmarshal(errorBody, &clickupErr); err != nil {
			return nil, fmt.Errorf("Error status: %d", resp.StatusCode)
		}

		if len(clickupErr.Err) > 0 {
			return nil, fmt.Errorf("ClickUp error: %s", clickupErr.Err)
		}

		return nil, fmt.Errorf("API error status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var clickUpResp ClickUpTasks
	if err := json.Unmarshal(body, &clickUpResp); err != nil {
		return nil, err
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
		tasks[i] = models.Task{
			Id:        clickUpTask.Id,
			Name:      clickUpTask.Name,
			Status:    clickUpTask.Status.Status,
			Assignees: assignees,
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
	}

	url := c.baseUrl + "/list/" + listId + "/task"

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("Error trying to parse body to Json: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))

	req.Header.Set("Authorization", c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("Error trying to read the body: %w", err)
		}

		var clickupErr ClickUpErrors

		if err := json.Unmarshal(errorBody, &clickupErr); err != nil {
			return nil, fmt.Errorf("Error status: %d", resp.StatusCode)
		}

		if len(clickupErr.Err) > 0 {
			return nil, fmt.Errorf("ClickUp error: %s", clickupErr.Err)
		}

		return nil, fmt.Errorf("API error status: %d", resp.StatusCode)
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var createdTask ClickUpTask
	if err := json.Unmarshal(responseBody, &createdTask); err != nil {
		return nil, fmt.Errorf("Error trying to parse resp: %w", err)
	}

	result := &models.Task{
		Id: createdTask.Id,
		Name: createdTask.Name,
		Status: createdTask.Status.Status,
	}

	return result, nil
}

func (c *ClickUpClient) GetWorkspaces() ([]ClickUpTeams, error) {
	url := c.baseUrl + "/team"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return []ClickUpTeams{}, err
	}

	req.Header.Set("Authorization", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return []ClickUpTeams{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return []ClickUpTeams{}, fmt.Errorf("Error trying to read the body: %w", err)
		}

		var clickupErr ClickUpErrors

		if err := json.Unmarshal(errorBody, &clickupErr); err != nil {
			return []ClickUpTeams{}, fmt.Errorf("Error trying to parse resp: %w", err)
		}
		
		if len(clickupErr.Err) > 0 {
			return []ClickUpTeams{}, fmt.Errorf("ClickUp error: %s", clickupErr.Err)
		}
		
		return []ClickUpTeams{}, fmt.Errorf("API error status: %d", resp.StatusCode)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return []ClickUpTeams{}, err
	}
	
	var clickupResp GetMultipleWorkspacesResponse
	if err := json.Unmarshal(body, &clickupResp); err != nil {
		return []ClickUpTeams{}, err
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
