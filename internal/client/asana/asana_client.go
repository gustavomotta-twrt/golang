package asana

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

func (c *AsanaClient) GetTasks(projectId string) ([]models.Task, error) {
	url := c.baseUrl + "/tasks?project=" + projectId + "&opt_fields=name,notes,completed"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

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

		var asanaErr AsanaErrors

		if err := json.Unmarshal(errorBody, &asanaErr); err != nil {
			return nil, fmt.Errorf("Error status: %d", resp.StatusCode)
		}

		if len(asanaErr.Errors) > 0 {
			return nil, fmt.Errorf("Asana error: %s", asanaErr.Errors[0].Message)
		}

		return nil, fmt.Errorf("API error status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var asanaResp AsanaResponse[AsanaTasks]
	if err := json.Unmarshal(body, &asanaResp); err != nil {
		return nil, err
	}

	tasks := make([]models.Task, len(asanaResp.Data))
	for i, asanaTask := range asanaResp.Data {
		status := "Incomplete"
		if asanaTask.Completed {
			status = "Completed"
		}
		tasks[i] = models.Task{
			Id:          asanaTask.Gid,
			Name:        asanaTask.Name,
			Description: asanaTask.Notes,
			Status:      status,
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
	}

	wrapper := CreateTaskRequestWrapper{
		Data: reqBody,
	}

	url := c.baseUrl + "/tasks"

	body, err := json.Marshal(wrapper)
	if err != nil {
		return nil, fmt.Errorf("Error trying to parse body to Json: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		errorBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("Error tryint to read the body: %w", err)
		}

		var asanaErr AsanaErrors

		if err := json.Unmarshal(errorBody, &asanaErr); err != nil {
			return nil, fmt.Errorf("Error status: %d", resp.StatusCode)
		}

		if len(asanaErr.Errors) > 0 {
			return nil, fmt.Errorf("Asana error: %s", asanaErr.Errors[0].Message)
		}

		return nil, fmt.Errorf("API error status: %d", resp.StatusCode)
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var createdTaskResp CreateTaskResponse
	if err := json.Unmarshal(responseBody, &createdTaskResp); err != nil {
		return nil, fmt.Errorf("Error trying to parse resp: %w", err)
	}

	status := "Incomplete"
	if createdTaskResp.Data.Completed {
		status = "Completed"
	}

	result := &models.Task{
		Id:          createdTaskResp.Data.Gid,
		Name:        createdTaskResp.Data.Name,
		Description: createdTaskResp.Data.Notes,
		Status:      status,
		Completed:   createdTaskResp.Data.Completed,
	}

	return result, nil
}

func (c *AsanaClient) GetWorkspaces() ([]GetMultipleWorkspacesResponse, error) {
	url := c.baseUrl + "/workspaces"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return []GetMultipleWorkspacesResponse{}, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return []GetMultipleWorkspacesResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return []GetMultipleWorkspacesResponse{}, fmt.Errorf("Error trying to read the body: %w", err)
		}

		var asanaErr AsanaErrors

		if err := json.Unmarshal(errorBody, &asanaErr); err != nil {
			return []GetMultipleWorkspacesResponse{}, fmt.Errorf("Error trying to parse resp: %w", err)
		}

		if len(asanaErr.Errors) > 0 {
			return []GetMultipleWorkspacesResponse{}, fmt.Errorf("Asana error: %s", asanaErr.Errors[0].Message)
		}

		return []GetMultipleWorkspacesResponse{}, fmt.Errorf("API error status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return []GetMultipleWorkspacesResponse{}, err
	}

	var asanaResp AsanaResponse[GetMultipleWorkspacesResponse]
	if err := json.Unmarshal(body, &asanaResp); err != nil {
		return []GetMultipleWorkspacesResponse{}, err
	}

	return asanaResp.Data, nil
}

func (c *AsanaClient) GetProjects(workspaceId string) ([]GetMultipleProjectsResponse, error) {
	url := c.baseUrl + "/projects?workspace=" + workspaceId

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return []GetMultipleProjectsResponse{}, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return []GetMultipleProjectsResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return []GetMultipleProjectsResponse{}, fmt.Errorf("Error trying to read the body: %w", err)
		}

		var asanaErr AsanaErrors

		if err := json.Unmarshal(errorBody, &asanaErr); err != nil {
			return []GetMultipleProjectsResponse{}, fmt.Errorf("Error trying to parse resp: %w", err)
		}

		if len(asanaErr.Errors) > 0 {
			return []GetMultipleProjectsResponse{}, fmt.Errorf("Asana error: %s", asanaErr.Errors[0].Message)
		}
		return []GetMultipleProjectsResponse{}, fmt.Errorf("API error status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return []GetMultipleProjectsResponse{}, err
	}

	var asanaResp AsanaResponse[GetMultipleProjectsResponse]
	if err := json.Unmarshal(body, &asanaResp); err != nil {
		return []GetMultipleProjectsResponse{}, err
	}

	return asanaResp.Data, nil
}
