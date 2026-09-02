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

	"code.vikunja.io/api/pkg/config"
	githubmodule "code.vikunja.io/api/pkg/modules/github"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHumaGitHubConnection covers the /api/v2 GitHub connection routes against
// a stubbed GitHub API:
//
// Permission gradient — GitHubConnection.CanRead delegates to Project.CanRead
// (any share level), Can{Create,Update,Delete} to Project.CanUpdate. The same
// user walks every rung by switching the parent path:
//   - project 1  (owned by testuser1): can do everything; holds fixture connection #1
//   - project 9  (read share):  CAN list, CANNOT create/update/delete/sync (connection #2)
//   - project 10 (write share): CAN everything (connection #3)
//   - project 2  (no access): forbidden on everything (connection #4)
//
// Successful creates need a reachable GitHub: the stub answers repo validation,
// and the webhook registration is skipped because no public URL is configured
// (which also keeps the test hermetic — no outbound hook creation).
func TestHumaGitHubConnection(t *testing.T) {
	// Enabled is the config default now, but save/restore anyway so this test
	// can't leak an override into whatever runs after it.
	prevEnabled := config.GithubEnabled.GetBool()
	config.GithubEnabled.Set("true")
	t.Cleanup(func() {
		config.GithubEnabled.Set(strconv.FormatBool(prevEnabled))
	})

	stub := stubGitHubAPI(t)
	restore := githubmodule.SetBaseURLForTesting(stub.URL)
	t.Cleanup(restore)

	owned := webHandlerTestV2{
		user:     &testuser1,
		basePath: "/api/v2/projects/1/integrations/github/connections",
		idParam:  "connection",
		t:        t,
	}
	require.NoError(t, owned.ensureEnv())
	on := func(projectID string) *webHandlerTestV2 {
		return &webHandlerTestV2{
			user:     &testuser1,
			basePath: "/api/v2/projects/" + projectID + "/integrations/github/connections",
			idParam:  "connection",
			t:        t,
			e:        owned.e,
		}
	}
	readShared := on("9")
	writeShared := on("10")
	forbidden := on("2")

	connectionIDsFromReadAll := func(t *testing.T, body []byte) []int64 {
		t.Helper()
		var parsed struct {
			Items []struct {
				ID int64 `json:"id"`
			} `json:"items"`
		}
		require.NoError(t, json.Unmarshal(body, &parsed))
		ids := make([]int64, 0, len(parsed.Items))
		for _, c := range parsed.Items {
			ids = append(ids, c.ID)
		}
		return ids
	}

	t.Run("ReadAll", func(t *testing.T) {
		t.Run("Normal", func(t *testing.T) {
			rec, err := owned.testReadAllWithUser(nil, nil)
			require.NoError(t, err)
			ids := connectionIDsFromReadAll(t, rec.Body.Bytes())
			assert.ElementsMatch(t, []int64{1}, ids, "body: %s", rec.Body.String())
		})
		t.Run("Credentials are never exposed", func(t *testing.T) {
			rec, err := owned.testReadAllWithUser(nil, nil)
			require.NoError(t, err)
			assert.NotContains(t, rec.Body.String(), `ghp_fixture_token`)
			assert.NotContains(t, rec.Body.String(), `fixture-secret-1`)
		})
		t.Run("Read-only share can list", func(t *testing.T) {
			rec, err := readShared.testReadAllWithUser(nil, nil)
			require.NoError(t, err)
			ids := connectionIDsFromReadAll(t, rec.Body.Bytes())
			assert.ElementsMatch(t, []int64{2}, ids, "body: %s", rec.Body.String())
		})
		t.Run("Write share can list", func(t *testing.T) {
			rec, err := writeShared.testReadAllWithUser(nil, nil)
			require.NoError(t, err)
			ids := connectionIDsFromReadAll(t, rec.Body.Bytes())
			assert.ElementsMatch(t, []int64{3}, ids, "body: %s", rec.Body.String())
		})
		t.Run("Forbidden", func(t *testing.T) {
			_, err := forbidden.testReadAllWithUser(nil, nil)
			require.Error(t, err)
			assert.Equal(t, http.StatusForbidden, getHTTPErrorCode(err))
		})
	})

	t.Run("Create", func(t *testing.T) {
		t.Run("Normal", func(t *testing.T) {
			rec, err := owned.testCreateWithUser(nil, nil,
				`{"repo_owner":"fixtureowner","repo_name":"fixturerepo","token":"ghp_some_token"}`)
			require.NoError(t, err)
			assert.Equal(t, http.StatusCreated, rec.Code)
			assert.Contains(t, rec.Body.String(), `"repo_owner":"fixtureowner"`)
			assert.Contains(t, rec.Body.String(), `"project_id":1`)
			assert.NotContains(t, rec.Body.String(), `ghp_some_token`)
		})
		t.Run("Duplicate repo is rejected", func(t *testing.T) {
			// Wait out the async initial backfill so its DB writes can't race
			// the following subtests.
			githubmodule.WaitForBackfills()
			_, err := owned.testCreateWithUser(nil, nil,
				`{"repo_owner":"fixtureowner","repo_name":"fixturerepo","token":"ghp_some_token"}`)
			require.Error(t, err)
			assert.Equal(t, http.StatusBadRequest, getHTTPErrorCode(err))
		})
		t.Run("Unknown repo is rejected", func(t *testing.T) {
			_, err := owned.testCreateWithUser(nil, nil,
				`{"repo_owner":"ghost","repo_name":"missing","token":"ghp_some_token"}`)
			require.Error(t, err)
			assert.Equal(t, http.StatusNotFound, getHTTPErrorCode(err))
		})
		t.Run("Both installation and token is rejected", func(t *testing.T) {
			_, err := owned.testCreateWithUser(nil, nil,
				`{"repo_owner":"fixtureowner","repo_name":"fixturerepo","token":"ghp_some_token","installation_id":5}`)
			require.Error(t, err)
			assert.Equal(t, http.StatusBadRequest, getHTTPErrorCode(err))
		})
		t.Run("Read share cannot create", func(t *testing.T) {
			_, err := readShared.testCreateWithUser(nil, nil,
				`{"repo_owner":"fixtureowner","repo_name":"fixturerepo","token":"ghp_some_token"}`)
			require.Error(t, err)
			assert.Equal(t, http.StatusForbidden, getHTTPErrorCode(err))
		})
		t.Run("Forbidden", func(t *testing.T) {
			_, err := forbidden.testCreateWithUser(nil, nil,
				`{"repo_owner":"fixtureowner","repo_name":"fixturerepo","token":"ghp_some_token"}`)
			require.Error(t, err)
			assert.Equal(t, http.StatusForbidden, getHTTPErrorCode(err))
		})
	})

	t.Run("Update", func(t *testing.T) {
		t.Run("Normal - only enabled changes", func(t *testing.T) {
			// repo_owner/repo_name are part of the request schema (the same model
			// doubles as body), but only the enabled column is ever persisted.
			rec, err := owned.testUpdateWithUser(nil, map[string]string{"connection": "1"},
				`{"repo_owner":"ignored","repo_name":"ignored","enabled":false}`)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), `"enabled":false`)
			assert.Contains(t, rec.Body.String(), `"repo_owner":"vikunja"`, "repo is immutable but must survive the update")
		})
		t.Run("Read share cannot update", func(t *testing.T) {
			_, err := readShared.testUpdateWithUser(nil, map[string]string{"connection": "2"},
				`{"repo_owner":"ignored","repo_name":"ignored","enabled":false}`)
			require.Error(t, err)
			assert.Equal(t, http.StatusForbidden, getHTTPErrorCode(err))
		})
		t.Run("Forbidden", func(t *testing.T) {
			_, err := forbidden.testUpdateWithUser(nil, map[string]string{"connection": "4"},
				`{"repo_owner":"ignored","repo_name":"ignored","enabled":false}`)
			require.Error(t, err)
			assert.Equal(t, http.StatusForbidden, getHTTPErrorCode(err))
		})
	})

	t.Run("Delete", func(t *testing.T) {
		t.Run("Write share can delete", func(t *testing.T) {
			rec, err := writeShared.testDeleteWithUser(nil, map[string]string{"connection": "3"})
			require.NoError(t, err)
			assert.Equal(t, http.StatusNoContent, rec.Code)
		})
		t.Run("Read share cannot delete", func(t *testing.T) {
			_, err := readShared.testDeleteWithUser(nil, map[string]string{"connection": "2"})
			require.Error(t, err)
			assert.Equal(t, http.StatusForbidden, getHTTPErrorCode(err))
		})
		t.Run("Forbidden", func(t *testing.T) {
			_, err := forbidden.testDeleteWithUser(nil, map[string]string{"connection": "4"})
			require.Error(t, err)
			assert.Equal(t, http.StatusForbidden, getHTTPErrorCode(err))
		})
		t.Run("Nonexistent", func(t *testing.T) {
			_, err := owned.testDeleteWithUser(nil, map[string]string{"connection": "9999"})
			require.Error(t, err)
			assert.Equal(t, http.StatusNotFound, getHTTPErrorCode(err))
		})
	})

	t.Run("Sync", func(t *testing.T) {
		// Custom action path: POST .../connections/{id}/sync — not part of the
		// generic harness, driven through serve() directly.
		t.Run("Normal", func(t *testing.T) {
			rec, err := owned.serve(http.MethodPost, "/api/v2/projects/1/integrations/github/connections/1/sync", "")
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), `"created"`)
		})
		t.Run("Forbidden", func(t *testing.T) {
			_, err := forbidden.serve(http.MethodPost, "/api/v2/projects/2/integrations/github/connections/4/sync", "")
			require.Error(t, err)
			assert.Equal(t, http.StatusForbidden, getHTTPErrorCode(err))
		})
	})

	t.Run("Webhook receiver", func(t *testing.T) {
		// Unauthenticated by design: signature is the auth. A bad signature
		// must 401; a ping needs no signature at all.
		body := `{"action":"opened","number":1,"repository":{"full_name":"vikunja/api","name":"api","owner":{"login":"vikunja"}},"installation":{"id":0}}`
		_, err := serveGitHubWebhook(&owned, body, map[string]string{
			"X-GitHub-Event":      "issues",
			"X-GitHub-Delivery":   "webtest-1",
			"X-Hub-Signature-256": "sha256=0000000000000000000000000000000000000000000000000000000000000000",
		})
		require.Error(t, err)
		assert.Equal(t, http.StatusUnauthorized, getHTTPErrorCode(err))

		rec, err := serveGitHubWebhook(&owned, `{"zen":"Keep it simple."}`, map[string]string{
			"X-GitHub-Event":    "ping",
			"X-GitHub-Delivery": "webtest-2",
		})
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

// serveGitHubWebhook posts to the public webhook receiver with GitHub's
// delivery headers. It bypasses serve() because that helper always attaches a
// JWT and offers no way to set custom headers.
func serveGitHubWebhook(h *webHandlerTestV2, payload string, headers map[string]string) (*httptest.ResponseRecorder, error) {
	require.NoError(h.t, h.ensureEnv())
	req := httptest.NewRequest(http.MethodPost, "/api/v2/integrations/github/webhook", strings.NewReader(payload))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.e.ServeHTTP(rec, req)
	if rec.Code >= 400 {
		return rec, newV2Error(rec)
	}
	return rec, nil
}

// stubGitHubAPI answers the endpoints the connection routes hit: repo
// validation (only fixtureowner/fixturerepo exists) and issue listings for
// every known repo (fixture vikunja/api and the created fixtureowner one).
func stubGitHubAPI(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Reject unauthenticated calls so masked-credential regressions (syncing
		// with an empty token) fail loudly instead of silently succeeding.
		auth := r.Header.Get("Authorization")
		if auth != "Bearer ghp_fixture_token" && auth != "Bearer ghp_some_token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
			return
		}
		if r.URL.Path == "/repos/fixtureowner/fixturerepo" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":        1,
				"node_id":   "R_1",
				"full_name": "fixtureowner/fixturerepo",
				"name":      "fixturerepo",
				"owner":     map[string]any{"login": "fixtureowner"},
			})
			return
		}
		if strings.HasPrefix(r.URL.Path, "/repos/") && strings.HasSuffix(r.URL.Path, "/issues") {
			// Distinct node ids per repo: links have a unique index on node_id,
			// and the async backfill of the created connection races later
			// syncs against the same stub.
			repo := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/repos/"), "/issues")

			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id": 1, "node_id": "node_" + repo, "number": 1,
					"title": "Fix the login bug", "body": "Broken", "state": "open",
					"html_url": "https://github.com/" + repo + "/issues/1",
					"labels":   []map[string]any{{"name": "bug", "color": "ff0000"}},
					"user":     map[string]any{"login": "someuser"},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	t.Cleanup(server.Close)
	return server
}
