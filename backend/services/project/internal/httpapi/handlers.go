// Package httpapi wires the Project CRUD endpoints, matching the frontend
// contract in frontend/src/api/projects.ts. Routes are registered without the
// /api prefix; the Gateway strips /api, resolves the workspace context, and
// forwards it via X-Workspace-ID / X-Workspace-IDs.
//
// Scoping (design D8): every project carries workspace_id; reads filter by the
// session's workspace set and mutations reject rows outside it (404), so a
// tenant can never observe or touch another tenant's projects even if the
// Gateway were misconfigured.
package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/aaks/server/internal/httputil"
	"github.com/aaks/server/services/project/internal/store"
)

func (a *App) list(w http.ResponseWriter, r *http.Request) {
	ws := httputil.WorkspaceIDs(r)
	ps, err := a.store.List(r.Context(), ws)
	if err != nil {
		httputil.ServerError(w, a.log, "project.List", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, ps)
}

func (a *App) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := a.store.Get(r.Context(), id, httputil.WorkspaceIDs(r))
	if errors.Is(err, store.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		httputil.ServerError(w, a.log, "project.Get", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, p)
}

func (a *App) create(w http.ResponseWriter, r *http.Request) {
	ws := httputil.WorkspaceID(r)
	if ws == "" {
		httputil.Error(w, http.StatusBadRequest, "no workspace context")
		return
	}
	var in store.CreateInput
	if err := httputil.ReadJSON(r, &in); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if in.Name == "" || in.RepoSource == "" || in.RepoType == "" {
		httputil.Error(w, http.StatusBadRequest, "name, repo_source and repo_type are required")
		return
	}
	p, err := a.store.Create(r.Context(), ws, in)
	if err != nil {
		httputil.ServerError(w, a.log, "project.Create", err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, p)
}

func (a *App) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in store.UpdateInput
	if err := httputil.ReadJSON(r, &in); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	p, err := a.store.Update(r.Context(), id, httputil.WorkspaceIDs(r), in)
	if errors.Is(err, store.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		httputil.ServerError(w, a.log, "project.Update", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, p)
}

func (a *App) delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := a.store.Delete(r.Context(), id, httputil.WorkspaceIDs(r))
	if errors.Is(err, store.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		httputil.ServerError(w, a.log, "project.Delete", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// App holds handler dependencies.
type App struct {
	store *store.ProjectStore
	log   *slog.Logger
}
