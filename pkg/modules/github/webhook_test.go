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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/models"
)

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	body := []byte(`{"action":"opened"}`)
	valid := sign("secret", body)

	if !VerifySignature([]byte("secret"), body, valid) {
		t.Error("valid signature rejected")
	}
	if VerifySignature([]byte("wrong"), body, valid) {
		t.Error("wrong secret accepted")
	}
	if VerifySignature([]byte("secret"), []byte("tampered"), valid) {
		t.Error("tampered body accepted")
	}
	if VerifySignature([]byte("secret"), body, "sha256=0000") {
		t.Error("garbage signature accepted")
	}
	if VerifySignature([]byte("secret"), body, "") {
		t.Error("missing signature accepted")
	}
}

// deliveryBody builds a webhook payload for the stub repo. The number is
// always the stub's single issue.
func deliveryBody(action string) []byte {
	payload := map[string]any{
		"action": action,
		"number": 1,
		"repository": map[string]any{
			"id":        1,
			"full_name": testOwner + "/" + testRepo,
			"name":      testRepo,
			"owner":     map[string]any{"login": testOwner},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return raw
}

func TestHandleWebhook(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	stub := newStubGitHub()
	defer stub.server.Close()
	setBaseURL(t, stub.server.URL)

	s := db.NewSession()
	insertTestConnection(t, s)
	if err := s.Commit(); err != nil {
		t.Fatalf("commit: %s", err)
	}
	_ = s.Close()

	body := deliveryBody("opened")

	// A well-signed delivery for a connected repo syncs the item.
	if err := HandleWebhook(context.Background(), "issues", "delivery-1", sign("test-webhook-secret", body), body); err != nil {
		t.Fatalf("HandleWebhook: %s", err)
	}
	link := mustLink(t, "MDU6SXNzdWVfb25l")

	// The same delivery id again is a no-op even with a fresh payload.
	if err := HandleWebhook(context.Background(), "issues", "delivery-1", sign("test-webhook-secret", body), body); err != nil {
		t.Fatalf("replayed HandleWebhook: %s", err)
	}

	// A new delivery with a bad signature is rejected without syncing.
	stub.issues[0].Title = "Changed title"
	badBody := deliveryBody("edited")
	err := HandleWebhook(context.Background(), "issues", "delivery-2", sign("wrong-secret", badBody), badBody)
	if !IsErrInvalidSignature(err) {
		t.Fatalf("expected signature error, got %v", err)
	}

	// Sessions must not stay open across HandleWebhook calls — it opens its
	// own, and SQLite file locks serialize writers.
	task := taskTitle(t, link.TaskID)
	if task != "Fix the login bug" {
		t.Errorf("bad-signature delivery was synced: title is %q", task)
	}

	// A correctly signed follow-up delivery applies the change.
	goodBody := deliveryBody("edited")
	if err := HandleWebhook(context.Background(), "issues", "delivery-3", sign("test-webhook-secret", goodBody), goodBody); err != nil {
		t.Fatalf("good HandleWebhook: %s", err)
	}
	task = taskTitle(t, link.TaskID)
	if task != "Changed title" {
		t.Errorf("delivery not applied: title is %q", task)
	}
}

func taskTitle(t *testing.T, taskID int64) string {
	t.Helper()
	s := db.NewSession()
	defer func() {
		_ = s.Close()
	}()
	task := &models.Task{}
	has, err := s.ID(taskID).Get(task)
	if err != nil || !has {
		t.Fatalf("loading task %d: %v %v", taskID, has, err)
	}
	return task.Title
}

func TestHandleWebhook_IgnoresIrrelevant(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	stub := newStubGitHub()
	defer stub.server.Close()
	setBaseURL(t, stub.server.URL)

	s := db.NewSession()
	_ = insertTestConnection(t, s)
	if err := s.Commit(); err != nil {
		t.Fatalf("commit: %s", err)
	}
	_ = s.Close()

	cases := []struct {
		event   string
		payload string
	}{
		{"ping", `{}`},
		{"issue_comment", `{"action":"created"}`},
		{"push", `{"after":"abc"}`},
	}
	for _, c := range cases {
		body := []byte(c.payload)
		if err := HandleWebhook(context.Background(), c.event, "delivery-"+c.event, "", body); err != nil {
			t.Errorf("event %q: unexpected error: %s", c.event, err)
		}
	}

	// Unknown repos are acknowledged without error.
	unknown := []byte(`{"action":"opened","number":1,"repository":{"full_name":"someone/elsewhere","name":"elsewhere","owner":{"login":"someone"}}}`)
	if err := HandleWebhook(context.Background(), "issues", "delivery-unknown", sign("any", unknown), unknown); err != nil {
		t.Errorf("unknown repo delivery: unexpected error: %s", err)
	}
}

func TestHandleWebhook_RecordsSyncErrors(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	stub := newStubGitHub()
	defer stub.server.Close()
	setBaseURL(t, stub.server.URL)

	// Kill the issue endpoint so the re-fetch fails.
	stub.server.Config.Handler = http.NotFoundHandler()

	s := db.NewSession()
	conn := insertTestConnection(t, s)
	if err := s.Commit(); err != nil {
		t.Fatalf("commit: %s", err)
	}
	_ = s.Close()

	body := deliveryBody("opened")
	// Sync failures are swallowed (200 to GitHub) but recorded on the connection.
	if err := HandleWebhook(context.Background(), "issues", "delivery-err", sign("test-webhook-secret", body), body); err != nil {
		t.Fatalf("HandleWebhook: %s", err)
	}

	s2 := db.NewSession()
	defer func() {
		_ = s2.Close()
	}()
	reloaded := &models.GitHubConnection{}
	has, err := s2.ID(conn.ID).Get(reloaded)
	if err != nil || !has {
		t.Fatalf("reloading connection: %v %v", has, err)
	}
	if reloaded.LastSyncError == "" {
		t.Error("sync error not recorded on the connection")
	}
}

func mustLink(t *testing.T, nodeID string) *models.GitHubTaskLink {
	t.Helper()
	s := db.NewSession()
	defer func() {
		_ = s.Close()
	}()
	link, err := models.GetGitHubTaskLinkByNodeID(s, nodeID)
	if err != nil {
		t.Fatalf("loading link: %s", err)
	}
	if link == nil {
		t.Fatalf("no link for node %s", nodeID)
	}
	if link.LastSeenAt.IsZero() {
		t.Error("link LastSeenAt not set")
	}
	return link
}

func TestContentHash(t *testing.T) {
	base := &Issue{
		NodeID: "n", Title: "t", Body: "b", State: "open",
		Labels: []*Label{{Name: "l", Color: "ff0000"}},
	}
	changed := *base
	changed.Title = "other"
	if contentHash(base) == contentHash(&changed) {
		t.Error("title change not reflected in hash")
	}
	// Label order must not matter.
	shuffled := *base
	shuffled.Labels = []*Label{{Name: "x", Color: "00ff00"}, {Name: "l", Color: "ff0000"}}
	if contentHash(base) == contentHash(&shuffled) {
		t.Error("label change not reflected in hash")
	}
	reordered := *base
	reordered.Labels = []*Label{{Name: "l", Color: "ff0000"}, {Name: "x", Color: "00ff00"}}
	if contentHash(&shuffled) != contentHash(&reordered) {
		t.Error("hash depends on label order")
	}
	// Timestamps with different zones but the same instant hash equal.
	one := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	two := one.In(time.FixedZone("UTC+2", 2*60*60))
	a, b := *base, *base
	a.ClosedAt, b.ClosedAt = &one, &two
	if contentHash(&a) != contentHash(&b) {
		t.Error("hash not timezone-stable")
	}
}
