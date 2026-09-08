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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/modules/auth"
	"code.vikunja.io/api/pkg/user"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The swimlane overview edits and renders tasks straight from the global
// /tasks collection, so this pins the contract that the collection already
// populates labels and assignees on every task — round-tripping such a task
// through the update endpoint must not silently drop relations.
func TestTaskCollectionIncludesRelations(t *testing.T) {
	e, err := setupTestEnv()
	require.NoError(t, err)

	s := db.NewSession()
	defer s.Close()
	u, err := user.GetUserByID(s, 1)
	require.NoError(t, err)
	jwt, err := auth.NewUserJWTAuthtoken(u, "test-session-id")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks?filter=done+%3D+false&per_page=100&page=1", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+jwt)
	res := httptest.NewRecorder()
	e.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	body := res.Body.String()

	// Label #4 is attached to tasks 1, 2 and 35 via label_tasks.yml; it
	// must appear as a real object, not just an empty labels array key.
	assert.Contains(t, body, `"title":"Label #4`)

	// Task #30 is open and has assignees 1 and 2 via task_assignees.yml —
	// its assignees must serialize as populated objects, not an empty array.
	start := strings.Index(body, `"title":"task #30 with assignees"`)
	require.NotEqual(t, -1, start)
	chunk := body[start:]
	if end := strings.Index(chunk, `"title":"task #31`); end != -1 {
		chunk = chunk[:end]
	}
	assert.Contains(t, chunk, `"assignees":[{`)

	// Tasks #1 and #2 carry label #4 via label_tasks.yml.
	task1Start := strings.Index(body, `"title":"task #1"`)
	require.NotEqual(t, -1, task1Start)
	task1 := body[task1Start:]
	if end := strings.Index(task1, `"title":"task #2"`); end != -1 {
		task1 = task1[:end]
	}
	assert.Contains(t, task1, `"labels":[{`)
}
