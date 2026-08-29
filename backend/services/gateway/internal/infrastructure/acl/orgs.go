package acl

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aaks/server/internal/contracts/identity"
)

// TaskClient resolves the workspace that owns a task from the Workspace
// service.
type TaskClient struct {
	url string
	hc  *http.Client
	log *slog.Logger
}

// NewTaskClient builds the task ownership client.
func NewTaskClient(url string, log *slog.Logger) *TaskClient {
	return &TaskClient{
		url: strings.TrimSuffix(url, "/"),
		hc:  &http.Client{Timeout: 5 * time.Second}, log: log,
	}
}

// Workspace implements application.TaskWorkspaceClient.
func (c *TaskClient) Workspace(ctx context.Context, taskID identity.ID) (identity.ID, error) {
	var res struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := doGet(c.hc, c.log, ctx, c.url+"/internal/tasks/"+string(taskID)+"/workspace", nil, &res); err != nil {
		return "", err
	}
	if res.WorkspaceID == "" {
		return "", errEmptyWorkspace
	}
	return identity.ID(res.WorkspaceID), nil
}

var errEmptyWorkspace = &emptyWorkspaceError{}

type emptyWorkspaceError struct{}

func (emptyWorkspaceError) Error() string { return "task workspace is empty" }
