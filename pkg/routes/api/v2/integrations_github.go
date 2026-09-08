// Vikunja is a to-do list application to facilitate your life.
// Copyright 2018-present Vikunja and contributors. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package apiv2

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/models"
	githubmodule "code.vikunja.io/api/pkg/modules/github"
	"code.vikunja.io/api/pkg/web/handler"

	"github.com/danielgtaylor/huma/v2"
	"github.com/labstack/echo/v5"
)

// githubConnectionListBody is the list response; credentials are masked by the
// model before rows are returned.
type githubConnectionListBody struct {
	Body Paginated[*models.GitHubConnection]
}

// githubSyncBody reports what a manual sync run changed.
type githubSyncBody struct {
	Body githubmodule.SyncStats
}

// GitHubWebhookHandler is the raw Echo handler for GitHub webhook deliveries.
// It is deliberately not a Huma operation: GitHub posts a different JSON shape
// per event type, and Huma's request validation would reject payloads that
// match no schema — the same reason the WebSocket upgrade stays outside Huma.
// Registered in routes.go on the v2 group, gated on github.enabled; the path
// is exempted from JWT auth there because deliveries authenticate via HMAC.
func GitHubWebhookHandler(c *echo.Context) error {
	body, err := io.ReadAll(io.LimitReader(c.Request().Body, 16<<20))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "could not read body")
	}

	err = githubmodule.HandleWebhook(c.Request().Context(),
		c.Request().Header.Get("X-GitHub-Event"),
		c.Request().Header.Get("X-GitHub-Delivery"),
		c.Request().Header.Get("X-Hub-Signature-256"),
		body)
	if githubmodule.IsErrInvalidSignature(err) {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid signature")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// RegisterGitHubIntegrationRoutes wires the GitHub integration onto the Huma
// API. Gated by github.enabled, checked here because RegisterAll runs after
// config is loaded.
func RegisterGitHubIntegrationRoutes(api huma.API) {
	if !config.GithubEnabled.GetBool() {
		return
	}

	tags := []string{"integrations"}

	Register(api, huma.Operation{
		OperationID: "github-connections-list",
		Summary:     "List a project's GitHub connections",
		Description: "Returns the GitHub repositories connected to the given project. Requires read access to the project. Tokens and webhook secrets are never included.",
		Method:      http.MethodGet,
		Path:        "/projects/{project}/integrations/github/connections",
		Tags:        tags,
	}, githubConnectionsList)

	Register(api, huma.Operation{
		OperationID: "github-connections-create",
		Summary:     "Connect a GitHub repository to a project",
		Description: "Connects a GitHub repository so its issues and pull requests are mirrored as tasks into the given project. Authenticate either with a GitHub App installation id (requires the instance's app credentials) or a personal access token with read access to the repository. The repository is validated immediately; the initial backfill runs in the background. Requires write access to the project.",
		Method:      http.MethodPost,
		Path:        "/projects/{project}/integrations/github/connections",
		Tags:        tags,
	}, githubConnectionsCreate)

	Register(api, huma.Operation{
		OperationID: "github-connections-update",
		Summary:     "Pause or resume a GitHub connection",
		Description: "Toggles a connection's enabled flag — the only mutable field. Disabled connections are skipped by all syncs but keep their mirrored tasks. The connection must belong to the project in the path, and write access to that project is required.",
		Method:      http.MethodPut,
		Path:        "/projects/{project}/integrations/github/connections/{connection}",
		Tags:        tags,
	}, githubConnectionsUpdate)

	Register(api, huma.Operation{
		OperationID: "github-connections-delete",
		Summary:     "Disconnect a GitHub repository",
		Description: "Removes the connection and its GitHub webhook (when Vikunja created one). Mirrored tasks and their links are kept — reconnecting the repository later resumes without duplicates. The connection must belong to the project in the path, and write access to that project is required.",
		Method:      http.MethodDelete,
		Path:        "/projects/{project}/integrations/github/connections/{connection}",
		Tags:        tags,
	}, githubConnectionsDelete)

	Register(api, huma.Operation{
		OperationID: "github-connections-sync",
		Summary:     "Sync a GitHub connection now",
		Description: "Runs a full reconciliation of the connection immediately instead of waiting for the scheduled sync: new and changed issues and pull requests are mirrored. Requires write access to the project.",
		Method:      http.MethodPost,
		Path:        "/projects/{project}/integrations/github/connections/{connection}/sync",
		Tags:        tags,
		// POST defaults to 201, but a sync doesn't create the resource at this
		// path — it triggers an action on it.
		DefaultStatus: http.StatusOK,
	}, githubConnectionsSync)
}

func init() { AddRouteRegistrar(RegisterGitHubIntegrationRoutes) }

func githubConnectionsList(ctx context.Context, in *struct {
	ProjectID int64 `path:"project"`
	ListParams
}) (*githubConnectionListBody, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	result, _, total, err := handler.DoReadAll(ctx, &models.GitHubConnection{ProjectID: in.ProjectID}, a, in.Q, in.Page, in.PerPage)
	if err != nil {
		return nil, translateDomainError(err)
	}
	items, ok := result.([]*models.GitHubConnection)
	if !ok {
		return nil, fmt.Errorf("GitHubConnection.ReadAll returned unexpected type %T (expected []*models.GitHubConnection)", result)
	}
	return &githubConnectionListBody{Body: NewPaginated(items, total, in.Page, in.PerPage)}, nil
}

func githubConnectionsCreate(ctx context.Context, in *struct {
	ProjectID int64 `path:"project"`
	Body      models.GitHubConnection
}) (*singleBody[models.GitHubConnection], error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	s := db.NewSession()
	defer func() {
		_ = s.Close()
	}()

	// Check permissions before anything touches the GitHub API — unauthorized
	// users must not cause external requests.
	canCreate := &models.GitHubConnection{ProjectID: in.ProjectID}
	can, err := canCreate.CanCreate(s, a)
	if err != nil {
		_ = s.Rollback()
		return nil, translateDomainError(err)
	}
	if !can {
		_ = s.Rollback()
		return nil, huma.Error403Forbidden("forbidden")
	}

	// Connect validates credentials + repo, creates the row and commits the
	// session itself (the background backfill needs the row persisted).
	conn, err := githubmodule.Connect(ctx, s, a, in.ProjectID, in.Body.InstallationID, in.Body.Token, in.Body.RepoOwner, in.Body.RepoName)
	if err != nil {
		_ = s.Rollback()
		return nil, translateDomainError(err)
	}
	return &singleBody[models.GitHubConnection]{Body: conn}, nil
}

func githubConnectionsUpdate(ctx context.Context, in *struct {
	ProjectID int64 `path:"project"`
	ID        int64 `path:"connection"`
	Body      models.GitHubConnection
}) (*singleBody[models.GitHubConnection], error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	in.Body.ID = in.ID
	in.Body.ProjectID = in.ProjectID
	if err := handler.DoUpdate(ctx, &in.Body, a); err != nil {
		return nil, translateDomainError(err)
	}
	return &singleBody[models.GitHubConnection]{Body: &in.Body}, nil
}

func githubConnectionsDelete(ctx context.Context, in *struct {
	ProjectID int64 `path:"project"`
	ID        int64 `path:"connection"`
}) (*emptyBody, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	s := db.NewSession()
	defer func() {
		_ = s.Close()
	}()

	// Resolve the row first so a missing connection 404s instead of blurring
	// into a 403, and the URL's project must match the row's parent. Credentials
	// stay intact: Disconnect needs the token to remove the repo webhook (the
	// response never includes them — Disconnect only deletes).
	full, err := models.GetGitHubConnectionByIDWithCredentials(s, in.ID)
	if err != nil {
		_ = s.Rollback()
		return nil, translateDomainError(err)
	}
	if full.ProjectID != in.ProjectID {
		_ = s.Rollback()
		return nil, huma.Error404NotFound("not found")
	}

	can, err := (&models.GitHubConnection{ID: in.ID, ProjectID: in.ProjectID}).CanDelete(s, a)
	if err != nil {
		_ = s.Rollback()
		return nil, translateDomainError(err)
	}
	if !can {
		_ = s.Rollback()
		return nil, huma.Error403Forbidden("forbidden")
	}

	// Disconnect needs the stored token to remove the repo webhook, which is
	// why it operates on the full row rather than the caller's copy.
	if err := githubmodule.Disconnect(ctx, s, full); err != nil {
		_ = s.Rollback()
		return nil, translateDomainError(err)
	}
	if err := s.Commit(); err != nil {
		return nil, translateDomainError(err)
	}
	return &emptyBody{}, nil
}

func githubConnectionsSync(ctx context.Context, in *struct {
	ProjectID int64 `path:"project"`
	ID        int64 `path:"connection"`
}) (*githubSyncBody, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	s := db.NewSession()
	defer func() {
		_ = s.Close()
	}()

	conn := &models.GitHubConnection{ID: in.ID, ProjectID: in.ProjectID}
	// Credentials intact: the sync client must authenticate against GitHub
	// with the stored token (a masked load would sync with an empty one).
	full, err := models.GetGitHubConnectionByIDWithCredentials(s, in.ID)
	if err != nil {
		_ = s.Rollback()
		return nil, translateDomainError(err)
	}
	if full.ProjectID != in.ProjectID {
		_ = s.Rollback()
		return nil, huma.Error404NotFound("not found")
	}

	can, err := conn.CanUpdate(s, a)
	if err != nil {
		_ = s.Rollback()
		return nil, translateDomainError(err)
	}
	if !can {
		_ = s.Rollback()
		return nil, huma.Error403Forbidden("forbidden")
	}

	stats, err := githubmodule.SyncNow(ctx, s, full)
	if err != nil {
		return nil, translateDomainError(err)
	}
	if stats == nil {
		// Another sync for this connection is already running.
		stats = &githubmodule.SyncStats{}
	}
	return &githubSyncBody{Body: *stats}, nil
}
