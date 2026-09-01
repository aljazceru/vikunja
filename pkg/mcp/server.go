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

package mcp

import (
	"context"
	"fmt"
	"net/http"

	"code.vikunja.io/api/pkg/log"
	"code.vikunja.io/api/pkg/version"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const serverInstructions = `This server manages work in Vikunja, a todo application. You act as an agent with access limited to specific projects.

Standard workflow for picking up work:
1. Call whoami to see your identity and which projects you can access.
2. Call list_tasks with the project id (and done=false) to find unassigned or assigned-to-you tasks.
3. When you start a task, call assign_to_me — it assigns the task to you and moves it to the project's "In Progress" bucket so humans see you are working on it.
4. Post progress reports with add_comment.
5. When finished, call complete_task and summarize what you did in a final comment.

Only touch tasks in projects you have access to; everything else will fail with a permission error. Never invent task or project ids — look them up first.`

// Handler returns the http.Handler serving the MCP streamable-http endpoint.
// A fresh server is built per request so tools/list reflects exactly the
// authenticated token's permissions.
func Handler() http.Handler {
	streamable := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return newServer(AuthFromContext(r.Context()))
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, err := authenticate(r)
		if err != nil {
			log.Errorf("[mcp auth] token validation error: %s", err)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if a == nil {
			http.Error(w, "unauthorized: a valid API token with mcp access is required", http.StatusUnauthorized)
			return
		}
		streamable.ServeHTTP(w, r.WithContext(
			context.WithValue(r.Context(), authContextKey{}, a)))
	})
}

func newServer(a *Auth) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "vikunja",
		Version: version.Version,
	}, &mcp.ServerOptions{Instructions: serverInstructions})

	registerWhoamiTool(srv, a)
	registerProjectTools(srv, a)
	registerTaskTools(srv, a)
	registerWorkflowTools(srv, a)
	return srv
}

// toolError turns any model error into a clean error string for the calling
// agent — MCP packs returned errors into the tool result instead of failing
// the HTTP request.
func toolError(format string, err error) error {
	if err == nil {
		return fmt.Errorf("%s", format)
	}
	return fmt.Errorf("%s: %s", format, err.Error())
}
