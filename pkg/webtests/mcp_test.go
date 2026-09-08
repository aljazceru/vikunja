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

package webtests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/user"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMCP drives the MCP endpoint end to end as a real agent client would:
// provision an agent, create an "In Progress" bucket, then claim a task over
// MCP and verify it moved — the exact chain the swimlane visibility relies on.
func TestMCP(t *testing.T) {
	e, err := setupTestEnv()
	require.NoError(t, err)

	u := user.User{ID: 1}
	jwt := humaTokenFor(t, &u)

	// An "In Progress" bucket for project 1's kanban view (fixture view 4).
	s := db.NewSession()
	inProgress := &models.Bucket{
		Title:         "In Progress",
		ProjectViewID: 4,
	}
	require.NoError(t, inProgress.Create(s, &u))
	require.NoError(t, s.Commit())

	// Provision the agent over the HTTP API, like an operator would.
	agentBody := `{"name":"MCP Worker","preset":"read-write","projects":[{"project_id":1,"permission":1}]}`
	rec := humaRequest(t, e, http.MethodPost, "/api/v2/agents", agentBody, jwt, "")
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct {
		Agent struct {
			ID int64 `json:"id"`
		} `json:"agent"`
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	agentToken := created.Token

	rpc := func(id int, method, params string) *httptest.ResponseRecorder {
		t.Helper()
		if params == "" {
			params = "{}"
		}
		body := `{"jsonrpc":"2.0","id":` + strconv.Itoa(id) + `,"method":"` + method + `","params":` + params + `}`
		req := httptest.NewRequest(http.MethodPost, "/api/v2/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+agentToken)
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)
		return w
	}

	t.Run("auth required", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v2/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("initialize", func(t *testing.T) {
		w := rpc(1, "initialize", `{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var res struct {
			Result struct {
				ServerInfo struct {
					Name string `json:"name"`
				} `json:"serverInfo"`
			} `json:"result"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
		assert.Equal(t, "vikunja", res.Result.ServerInfo.Name)
	})

	t.Run("tools list reflects read-write preset", func(t *testing.T) {
		w := rpc(2, "tools/list", "")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var res struct {
			Result struct {
				Tools []struct {
					Name string `json:"name"`
				} `json:"tools"`
			} `json:"result"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
		names := map[string]bool{}
		for _, tool := range res.Result.Tools {
			names[tool.Name] = true
		}
		for _, expected := range []string{"whoami", "list_tasks", "get_task", "create_task", "update_task", "assign_to_me", "complete_task", "add_comment", "set_task_in_progress"} {
			assert.True(t, names[expected], "tool %s should be listed", expected)
		}
	})

	t.Run("whoami", func(t *testing.T) {
		w := rpc(3, "tools/call", `{"name":"whoami","arguments":{}}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var res struct {
			Result struct {
				StructuredContent struct {
					IsBot bool `json:"is_bot"`
					User  struct {
						ID int64 `json:"id"`
					} `json:"user"`
					Projects []struct {
						ID int64 `json:"id"`
					} `json:"projects"`
				} `json:"structuredContent"`
			} `json:"result"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
		assert.Equal(t, created.Agent.ID, res.Result.StructuredContent.User.ID)
		assert.True(t, res.Result.StructuredContent.IsBot)
		require.Len(t, res.Result.StructuredContent.Projects, 1)
		assert.Equal(t, int64(1), res.Result.StructuredContent.Projects[0].ID)
	})

	t.Run("claim moves task to in-progress bucket", func(t *testing.T) {
		// Task 1 lives in fixture bucket 1 of view 4.
		w := rpc(4, "tools/call", `{"name":"assign_to_me","arguments":{"task_id":1}}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.NotContains(t, w.Body.String(), `"isError":true`, w.Body.String())

		s := db.NewSession()
		defer s.Close()
		tb := &models.TaskBucket{}
		found, err := s.Where("task_id = ? AND project_view_id = ?", 1, 4).Get(tb)
		require.NoError(t, err)
		require.True(t, found, "task bucket row must exist")
		assert.Equal(t, inProgress.ID, tb.BucketID, "task must have moved to the In Progress bucket")

		// And it is assigned to the agent's bot user.
		count, err := s.Where("task_id = ? AND user_id = ?", 1, created.Agent.ID).Count(&models.TaskAssginee{})
		require.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})

	t.Run("comment and complete", func(t *testing.T) {
		w := rpc(5, "tools/call", `{"name":"add_comment","arguments":{"task_id":1,"comment":"work in progress, investigating"}}`)
		require.Equal(t, http.StatusOK, w.Code)
		assert.NotContains(t, w.Body.String(), `"isError":true`, w.Body.String())

		w = rpc(6, "tools/call", `{"name":"complete_task","arguments":{"task_id":1}}`)
		require.Equal(t, http.StatusOK, w.Code)
		assert.NotContains(t, w.Body.String(), `"isError":true`, w.Body.String())

		s := db.NewSession()
		defer s.Close()
		task := &models.Task{ID: 1}
		require.NoError(t, task.ReadOne(s, &u))
		assert.True(t, task.Done)
	})

	t.Run("out-of-scope project is a tool error", func(t *testing.T) {
		// A task in a project the agent is not a member of must fail as a
		// tool error, not an HTTP error — the MCP session stays usable.
		var foreignTaskID int64
		s := db.NewSession()
		foreign := &models.Task{}
		exists, err := s.Where("project_id = ?", 5).Get(foreign)
		require.NoError(t, err)
		require.True(t, exists, "fixture project 5 must have tasks")
		foreignTaskID = foreign.ID
		require.NoError(t, s.Close())

		w := rpc(7, "tools/call", `{"name":"assign_to_me","arguments":{"task_id":`+strconv.FormatInt(foreignTaskID, 10)+`}}`)
		require.Equal(t, http.StatusOK, w.Code, "MCP tool errors come back in the result, not as HTTP errors")
		assert.Contains(t, w.Body.String(), `"isError":true`)
	})
}
