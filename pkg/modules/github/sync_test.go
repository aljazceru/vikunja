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

package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/models"

	"xorm.io/xorm"
)

const (
	testOwner = "fixtureowner"
	testRepo  = "fixturerepo"
	// testToken is the PAT insertTestConnection stores; the stub rejects
	// requests without it so credential-masking regressions fail loudly.
	testToken = "ghp_testtoken"
)

// stubGitHub spins up a GitHub API stub serving an issue and a PR referencing
// it. mutate can rewrite the served items between syncs.
type stubGitHub struct {
	server *httptest.Server
	issues []*Issue
}

func newStubGitHub() *stubGitHub {
	stub := &stubGitHub{
		issues: []*Issue{
			{
				ID:      1,
				NodeID:  "MDU6SXNzdWVfb25l",
				Number:  1,
				Title:   "Fix the login bug",
				Body:    "Logging in is broken on mobile.",
				State:   "open",
				HTMLURL: "https://github.com/" + testOwner + "/" + testRepo + "/issues/1",
				Labels:  []*Label{{Name: "bug", Color: "ff0000"}},
				User:    &User{Login: "someuser"},
			},
			{
				ID:          2,
				NodeID:      "PR_kwDObmFkZXR3bw",
				Number:      2,
				Title:       "Actually fix the login bug",
				Body:        "Fixes #1",
				State:       "open",
				HTMLURL:     "https://github.com/" + testOwner + "/" + testRepo + "/pull/2",
				Labels:      []*Label{},
				User:        &User{Login: "someuser"},
				PullRequest: &PullRequestRef{HTMLURL: "https://github.com/" + testOwner + "/" + testRepo + "/pull/2"},
			},
		},
	}

	mux := http.NewServeMux()
	requireAuth := func(r *http.Request) bool {
		return r.Header.Get("Authorization") == "Bearer "+testToken
	}
	mux.HandleFunc("/repos/"+testOwner+"/"+testRepo+"/issues", func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stub.issues)
	})
	// The sync engine re-fetches single items by number through the list path
	// shape: /issues/{number}.
	mux.HandleFunc("/repos/"+testOwner+"/"+testRepo+"/issues/", func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stub.issues[0])
	})
	mux.HandleFunc("/repos/"+testOwner+"/"+testRepo, func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Repository{
			ID:       1,
			NodeID:   "R_1",
			FullName: testOwner + "/" + testRepo,
			Owner:    &User{Login: testOwner},
			Name:     testRepo,
		})
	})
	stub.server = httptest.NewServer(mux)
	return stub
}

// insertTestConnection persists a PAT connection the sync can authenticate
// with (GetGitHubConnectionByID returns a masked copy, unsuitable for syncing).
func insertTestConnection(t *testing.T, s *xorm.Session) *models.GitHubConnection {
	t.Helper()
	conn := &models.GitHubConnection{
		ProjectID:     1,
		RepoOwner:     testOwner,
		RepoName:      testRepo,
		Token:         testToken,
		WebhookSecret: "test-webhook-secret",
		Enabled:       true,
		CreatedByID:   1,
	}
	_, err := s.Insert(conn)
	if err != nil {
		t.Fatalf("inserting test connection: %s", err)
	}
	return conn
}

func TestSyncConnection(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	stub := newStubGitHub()
	defer stub.server.Close()

	s := db.NewSession()
	defer func() {
		_ = s.Close()
	}()
	conn := insertTestConnection(t, s)
	// Point the client at the stub instead of api.github.com.
	oldDefault := defaultBaseURL
	setBaseURL(t, stub.server.URL)

	stats, err := SyncConnection(context.Background(), s, conn)
	if err != nil {
		t.Fatalf("SyncConnection: %s", err)
	}
	if err := s.Commit(); err != nil {
		t.Fatalf("commit: %s", err)
	}
	defer setBaseURL(t, oldDefault)

	if stats.Created != 2 {
		t.Fatalf("expected 2 created tasks, got %+v", stats)
	}

	// Both items exist as tasks in project 1.
	tasks := []models.Task{}
	if err := s.Where("project_id = ?", 1).In("id", []int64{taskIDFor(t, s, "MDU6SXNzdWVfb25l"), taskIDFor(t, s, "PR_kwDObmFkZXR3bw")}).Find(&tasks); err != nil {
		t.Fatalf("loading tasks: %s", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 mirrored tasks, got %d", len(tasks))
	}
	for _, task := range tasks {
		if task.Done {
			t.Errorf("open item mirrored as done: %q", task.Title)
		}
		if task.Description == "" {
			t.Errorf("task %q has no description", task.Title)
		}
	}

	// The issue carries the GitHub label, the repo label and the issue label;
	// the PR carries repo + pr labels.
	assertTaskLabels(t, s, taskIDFor(t, s, "MDU6SXNzdWVfb25l"), []string{"gh:bug", "repo:" + testOwner + "/" + testRepo, "gh:issue"})
	assertTaskLabels(t, s, taskIDFor(t, s, "PR_kwDObmFkZXR3bw"), []string{"repo:" + testOwner + "/" + testRepo, "gh:pr"})

	// "Fixes #1" produced a related relation between PR and issue.
	relExists, err := s.
		Where("(task_id = ? AND other_task_id = ?) OR (task_id = ? AND other_task_id = ?)",
			taskIDFor(t, s, "MDU6SXNzdWVfb25l"), taskIDFor(t, s, "PR_kwDObmFkZXR3bw"),
			taskIDFor(t, s, "PR_kwDObmFkZXR3bw"), taskIDFor(t, s, "MDU6SXNzdWVfb25l")).
		Exist(&models.TaskRelation{RelationKind: models.RelationKindRelated})
	if err != nil {
		t.Fatalf("checking relation: %s", err)
	}
	if !relExists {
		t.Error("expected a related relation between PR and issue task")
	}

	// The connection recorded the successful sync.
	reloaded := &models.GitHubConnection{}
	has, err := s.ID(conn.ID).Get(reloaded)
	if err != nil || !has {
		t.Fatalf("reloading connection: %v %v", has, err)
	}
	if reloaded.LastSyncedAt.IsZero() {
		t.Error("LastSyncedAt not set after sync")
	}
	if reloaded.LastSyncError != "" {
		t.Errorf("LastSyncError set after successful sync: %q", reloaded.LastSyncError)
	}

	// Re-running is a no-op: same items, no duplicates.
	stats, err = SyncConnection(context.Background(), s, reloaded)
	if err != nil {
		t.Fatalf("second SyncConnection: %s", err)
	}
	if err := s.Commit(); err != nil {
		t.Fatalf("commit: %s", err)
	}
	if stats.Unchanged != 2 || stats.Created != 0 || stats.Updated != 0 {
		t.Errorf("second sync not idempotent: %+v", stats)
	}
	taskCount(t, s, 2)
}

func TestSyncConnection_Updates(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	stub := newStubGitHub()
	defer stub.server.Close()

	s := db.NewSession()
	defer func() {
		_ = s.Close()
	}()
	conn := insertTestConnection(t, s)
	setBaseURL(t, stub.server.URL)

	if _, err := SyncConnection(context.Background(), s, conn); err != nil {
		t.Fatalf("initial sync: %s", err)
	}
	if err := s.Commit(); err != nil {
		t.Fatalf("commit: %s", err)
	}

	// The issue gets closed and loses its label; the label must disappear
	// from the task while the task itself is marked done, not deleted.
	closed := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	stub.issues[0].State = "closed"
	stub.issues[0].ClosedAt = &closed
	stub.issues[0].Labels = []*Label{}

	reloaded := &models.GitHubConnection{}
	has, err := s.ID(conn.ID).Get(reloaded)
	if err != nil || !has {
		t.Fatalf("reloading connection: %v %v", has, err)
	}
	if _, err := SyncConnection(context.Background(), s, reloaded); err != nil {
		t.Fatalf("update sync: %s", err)
	}
	if err := s.Commit(); err != nil {
		t.Fatalf("commit: %s", err)
	}

	issueTaskID := taskIDFor(t, s, "MDU6SXNzdWVfb25l")
	task := &models.Task{}
	has, err = s.ID(issueTaskID).Get(task)
	if err != nil || !has {
		t.Fatalf("loading mirrored task: %v %v", has, err)
	}
	if !task.Done {
		t.Error("closed issue not marked done")
	}
	if task.DoneAt.IsZero() {
		t.Error("DoneAt not set on closed issue")
	}
	assertTaskLabels(t, s, issueTaskID, []string{"repo:" + testOwner + "/" + testRepo, "gh:issue"})
	taskCount(t, s, 2)
}

func TestSyncConnection_ReopeningClearsDone(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	stub := newStubGitHub()
	defer stub.server.Close()

	s := db.NewSession()
	defer func() {
		_ = s.Close()
	}()
	conn := insertTestConnection(t, s)
	setBaseURL(t, stub.server.URL)

	closed := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	stub.issues[0].State = "closed"
	stub.issues[0].ClosedAt = &closed

	if _, err := SyncConnection(context.Background(), s, conn); err != nil {
		t.Fatalf("initial sync: %s", err)
	}
	if err := s.Commit(); err != nil {
		t.Fatalf("commit: %s", err)
	}

	// Reopen on GitHub: the task must be un-done with a cleared done_at —
	// this is why updates go through a map, not a struct.
	stub.issues[0].State = "open"
	stub.issues[0].ClosedAt = nil

	reloaded := &models.GitHubConnection{}
	_, _ = s.ID(conn.ID).Get(reloaded)
	if _, err := SyncConnection(context.Background(), s, reloaded); err != nil {
		t.Fatalf("reopen sync: %s", err)
	}
	if err := s.Commit(); err != nil {
		t.Fatalf("commit: %s", err)
	}

	task := &models.Task{}
	has, err := s.ID(taskIDFor(t, s, "MDU6SXNzdWVfb25l")).Get(task)
	if err != nil || !has {
		t.Fatalf("loading task: %v %v", has, err)
	}
	if task.Done {
		t.Error("reopened issue still done")
	}
	if !task.DoneAt.IsZero() {
		t.Errorf("DoneAt not cleared on reopen, got %v", task.DoneAt)
	}
}

// setBaseURL redirects the package's client construction at a stub server for
// the duration of the test.
func setBaseURL(t *testing.T, url string) {
	t.Helper()
	orig := defaultBaseURL
	defaultBaseURL = url
	t.Cleanup(func() {
		defaultBaseURL = orig
	})
}

func taskIDFor(t *testing.T, s *xorm.Session, nodeID string) int64 {
	t.Helper()
	link, err := models.GetGitHubTaskLinkByNodeID(s, nodeID)
	if err != nil {
		t.Fatalf("loading link for %s: %s", nodeID, err)
	}
	if link == nil {
		t.Fatalf("no link found for node %s", nodeID)
	}
	return link.TaskID
}

func assertTaskLabels(t *testing.T, s *xorm.Session, taskID int64, want []string) {
	t.Helper()
	rows := []models.Label{}
	err := s.
		Table("labels").
		Join("INNER", "label_tasks", "label_tasks.label_id = labels.id").
		Where("label_tasks.task_id = ?", taskID).
		Find(&rows)
	if err != nil {
		t.Fatalf("loading labels: %s", err)
	}
	got := map[string]bool{}
	for _, row := range rows {
		got[row.Title] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("task %d misses label %q, has %v", taskID, w, got)
		}
	}
}

func taskCount(t *testing.T, s *xorm.Session, want int) {
	t.Helper()
	// The fixtures ship one unrelated link; only count this repo's.
	count, err := s.Where("repo_owner = ? AND repo_name = ?", testOwner, testRepo).Count(&models.GitHubTaskLink{})
	if err != nil {
		t.Fatalf("counting links: %s", err)
	}
	if int(count) != want {
		t.Errorf("got %d linked tasks, want %d", count, want)
	}
}
