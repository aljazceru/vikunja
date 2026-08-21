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
	"strconv"
	"strings"
	"testing"

	"code.vikunja.io/api/pkg/user"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHumaAgent covers the one-step agent provisioning HTTP surface: create
// returns the agent plus a one-time token, list/read expose memberships, and
// other users are locked out. The MCP flow itself lives in TestMCP below.
func TestHumaAgent(t *testing.T) {
	e, err := setupTestEnv()
	require.NoError(t, err)

	user1 := user.User{ID: 1}
	token1 := humaTokenFor(t, &user1)
	user2 := user.User{ID: 2}
	token2 := humaTokenFor(t, &user2)

	t.Run("create with memberships", func(t *testing.T) {
		body := `{"name":"Research Agent","preset":"read-write","projects":[{"project_id":1,"permission":1}]}`
		rec := humaRequest(t, e, http.MethodPost, "/api/v2/agents", body, token1, "")
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

		var res struct {
			Agent struct {
				ID       int64 `json:"id"`
				Username string
				Projects []struct {
					ProjectID int64 `json:"project_id"`
					Title     string
				} `json:"projects"`
				Tokens []struct {
					ID int64 `json:"id"`
				} `json:"tokens"`
			} `json:"agent"`
			Token string `json:"token"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
		assert.True(t, strings.HasPrefix(res.Token, "tk_"), "one-time token must be returned")
		assert.True(t, strings.HasPrefix(res.Agent.Username, "bot-"))
		require.Len(t, res.Agent.Projects, 1)
		assert.Equal(t, int64(1), res.Agent.Projects[0].ProjectID)
		assert.NotEmpty(t, res.Agent.Projects[0].Title, "project title should be resolved")
		require.Len(t, res.Agent.Tokens, 1)
	})

	t.Run("list only own agents", func(t *testing.T) {
		rec := humaRequest(t, e, http.MethodGet, "/api/v2/agents", "", token1, "")
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "Research Agent")

		rec2 := humaRequest(t, e, http.MethodGet, "/api/v2/agents", "", token2, "")
		require.Equal(t, http.StatusOK, rec2.Code)
		assert.NotContains(t, rec2.Body.String(), "Research Agent")
	})

	t.Run("read refuses foreign agent", func(t *testing.T) {
		// Agent id 23's bot fixture is owned by user 21, not user 1.
		rec := humaRequest(t, e, http.MethodGet, "/api/v2/agents/23", "", token2, "")
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("invalid preset is rejected", func(t *testing.T) {
		rec := humaRequest(t, e, http.MethodPost, "/api/v2/agents", `{"name":"x","preset":"nope"}`, token1, "")
		// Huma's enum validation rejects the payload before the model sees it.
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("rotate and delete", func(t *testing.T) {
		body := `{"name":"Rot","preset":"read-only"}`
		rec := humaRequest(t, e, http.MethodPost, "/api/v2/agents", body, token1, "")
		require.Equal(t, http.StatusCreated, rec.Code)
		var created struct {
			Agent struct {
				ID int64 `json:"id"`
			} `json:"agent"`
			Token string `json:"token"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
		oldToken := created.Token

		rec = humaRequest(t, e, http.MethodPost, "/api/v2/agents/"+strconv.FormatInt(created.Agent.ID, 10)+"/rotate-token", `{}`, token1, "")
		require.Equal(t, http.StatusCreated, rec.Code)
		var rotated struct {
			Token string `json:"token"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rotated))
		assert.NotEqual(t, oldToken, rotated.Token)

		// The old token no longer authenticates anywhere.
		rec = humaRequest(t, e, http.MethodGet, "/api/v1/tasks", "", oldToken, "")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)

		rec = humaRequest(t, e, http.MethodDelete, "/api/v2/agents/"+strconv.FormatInt(created.Agent.ID, 10), "", token1, "")
		require.Equal(t, http.StatusNoContent, rec.Code)

		rec = humaRequest(t, e, http.MethodGet, "/api/v2/agents/"+strconv.FormatInt(created.Agent.ID, 10), "", token1, "")
		// The bot user is gone, so the agent is simply not found anymore.
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}
