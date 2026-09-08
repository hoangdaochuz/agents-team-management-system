package acl

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/platform/internaltoken"
	"github.com/aaks/server/services/gateway/internal/application"
)

// TaskClient resolves the workspace that owns a task from the Workspace
// service.
type TaskClient struct {
	url           string
	internalToken string
	hc            *http.Client
	log           *slog.Logger
}

// NewTaskClient builds the task ownership client.
func NewTaskClient(url, internalToken string, log *slog.Logger) *TaskClient {
	return &TaskClient{
		url: strings.TrimSuffix(url, "/"), internalToken: internalToken,
		hc: &http.Client{Timeout: 5 * time.Second}, log: log,
	}
}

// internalHeaders carries the shared service token on /internal/* calls.
func internalHeaders(token string) http.Header {
	if token == "" {
		return nil
	}
	return http.Header{internaltoken.Header: []string{token}}
}

// Workspace implements application.TaskWorkspaceClient. A definitive 404 maps
// to application.ErrTaskNotFound; anything else (transport, 5xx) propagates so
// the caller can distinguish "no such task" from "task service unavailable".
func (c *TaskClient) Workspace(ctx context.Context, taskID identity.ID) (identity.ID, error) {
	var res struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := doGet(c.hc, c.log, ctx, c.url+"/internal/tasks/"+string(taskID)+"/workspace", internalHeaders(c.internalToken), &res); err != nil {
		var se *StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return "", fmt.Errorf("%w: %s", application.ErrTaskNotFound, se.Error())
		}
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
