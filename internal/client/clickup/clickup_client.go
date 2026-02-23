package clickup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
		tasks[i] = models.Task{
			Id:     clickUpTask.Id,
			Name:   clickUpTask.Name,
			Status: clickUpTask.Status.Status,
		}
	}

	return tasks, nil
}

func (c *ClickUpClient) CreateTask(listId string, task models.Task) (*models.Task, error) {
	reqBody := CreateTaskRequest{
		Name:        task.Name,
		Description: task.Description,
		Status:      task.Status,
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
