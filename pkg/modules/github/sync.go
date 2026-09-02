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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/log"
	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/richtext"
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/utils"

	"xorm.io/xorm"
)

// Label prefixes namespace GitHub-generated labels so they never collide with
// a user's own labels and so label sync only ever touches its own labels.
const (
	issueLabelTitle       = "gh:issue"
	pullRequestLabelTitle = "gh:pr"
	repoLabelPrefix       = "repo:"
)

// SyncStats reports what a sync run changed.
type SyncStats struct {
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
}

// connectionLocks serializes syncs per connection so cron, webhook and manual
// syncs can't interleave on the same repo. Entries are never removed — the
// mutexes are tiny and connections are few.
var connectionLocks keyedMutexes

// keyedMutexes hands out one mutex per int64 key backed by a sync.Map.
type keyedMutexes struct {
	inner sync.Map // map[int64]*sync.Mutex
}

func (m *keyedMutexes) TryLock(key int64) bool {
	v, _ := m.inner.LoadOrStore(key, &sync.Mutex{})
	return v.(*sync.Mutex).TryLock()
}

func (m *keyedMutexes) Unlock(key int64) {
	v, ok := m.inner.Load(key)
	if !ok {
		return
	}
	v.(*sync.Mutex).Unlock()
}

// SyncConnection mirrors all GitHub items of the connection's repo that
// changed since the last successful sync into the project. A zero LastSyncedAt
// makes this a full backfill.
//
// The session is not committed — callers own the transaction.
func SyncConnection(ctx context.Context, s *xorm.Session, conn *models.GitHubConnection) (*SyncStats, error) {
	if !connectionLocks.TryLock(conn.ID) {
		return nil, nil // a sync for this connection is already running
	}
	defer connectionLocks.Unlock(conn.ID)

	client := NewClientForConnection(conn)

	actor, err := getSyncActor(s, conn)
	if err != nil {
		return nil, err
	}

	issues, err := client.ListIssues(ctx, conn.RepoOwner, conn.RepoName, conn.LastSyncedAt)
	if err != nil {
		return nil, err
	}

	stats := &SyncStats{}
	for _, issue := range issues {
		if err := syncIssue(s, client, actor, conn, issue, stats); err != nil {
			return stats, fmt.Errorf("syncing %s#%d: %w", conn.RepoFullName(), issue.Number, err)
		}
	}

	if _, err := s.ID(conn.ID).
		Cols("last_synced_at", "last_sync_error").
		Update(&models.GitHubConnection{
			LastSyncedAt:  time.Now(),
			LastSyncError: "",
		}); err != nil {
		return stats, err
	}
	return stats, nil
}

// SyncIssue mirrors one GitHub item, fetching its current state first. Used by
// the webhook receiver; the listing path calls syncIssue directly.
func SyncIssue(ctx context.Context, s *xorm.Session, client *Client, conn *models.GitHubConnection, number int64) (*SyncStats, error) {
	issue, err := client.GetIssue(ctx, conn.RepoOwner, conn.RepoName, number)
	if err != nil {
		return nil, err
	}
	actor, err := getSyncActor(s, conn)
	if err != nil {
		return nil, err
	}
	stats := &SyncStats{}
	if err := syncIssue(s, client, actor, conn, issue, stats); err != nil {
		return stats, err
	}
	return stats, nil
}

// syncIssue creates or updates the task for one GitHub item. Unchanged items
// (same content hash) only refresh their last-seen timestamp.
func syncIssue(s *xorm.Session, client *Client, actor *user.User, conn *models.GitHubConnection, issue *Issue, stats *SyncStats) error {
	hash := contentHash(issue)

	link, err := models.GetGitHubTaskLinkByNodeID(s, issue.NodeID)
	if err != nil {
		return err
	}

	if link != nil && link.ContentHash == hash {
		_, err = s.ID(link.ID).
			Cols("last_seen_at", "repo_owner", "repo_name", "html_url", "connection").
			Update(&models.GitHubTaskLink{
				LastSeenAt: time.Now(),
				RepoOwner:  conn.RepoOwner,
				RepoName:   conn.RepoName,
				HTMLURL:    issue.HTMLURL,
				Connection: conn.ID,
			})
		stats.Unchanged++
		return err
	}

	labelIDs, err := ensureLabels(s, actor, conn, issue)
	if err != nil {
		return err
	}

	var taskID int64
	if link == nil {
		taskID, err = createMirroredTask(s, actor, conn, issue, labelIDs)
		stats.Created++
	} else {
		taskID = link.TaskID
		err = updateMirroredTask(s, actor, link, issue, labelIDs)
		stats.Updated++
	}
	if err != nil {
		return err
	}

	if link == nil {
		link = &models.GitHubTaskLink{
			TaskID:    taskID,
			ProjectID: conn.ProjectID,
		}
	}
	link.Connection = conn.ID
	link.ItemType = itemTypeFor(issue)
	link.NodeID = issue.NodeID
	link.Number = issue.Number
	link.RepoOwner = conn.RepoOwner
	link.RepoName = conn.RepoName
	link.HTMLURL = issue.HTMLURL
	link.ContentHash = hash
	link.LastSeenAt = time.Now()

	if link.ID == 0 {
		_, err = s.Insert(link)
	} else {
		_, err = s.ID(link.ID).
			Cols("connection", "item_type", "node_id", "number", "repo_owner", "repo_name", "html_url", "content_hash", "last_seen_at").
			Update(link)
	}
	if err != nil {
		return err
	}

	return syncPullRequestRelations(s, actor, client, conn, issue, taskID)
}

func itemTypeFor(issue *Issue) string {
	if issue.IsPullRequest() {
		return "pull_request"
	}
	return "issue"
}

// buildDescription renders the GitHub body plus an attribution/link footer.
// Comments are deliberately not mirrored — discussions stay on GitHub.
func buildDescription(repo repoNamer, issue *Issue) string {
	kind := "Issue"
	if issue.IsPullRequest() {
		kind = "Pull request"
	}
	author := ""
	if issue.User != nil {
		author = " by @" + issue.User.Login
	}
	footer := fmt.Sprintf("---\n[%s #%d · %s](%s)%s",
		kind, issue.Number, repo.RepoFullName(), issue.HTMLURL, author)

	body := strings.TrimSpace(issue.Body)
	if body == "" {
		return footer
	}
	return body + "\n\n" + footer
}

func createMirroredTask(s *xorm.Session, actor *user.User, conn *models.GitHubConnection, issue *Issue, labelIDs []int64) (int64, error) {
	desc, err := richtext.MarkdownToHTML(buildDescription(conn, issue))
	if err != nil {
		return 0, fmt.Errorf("converting description: %w", err)
	}

	task := &models.Task{
		ProjectID:   conn.ProjectID,
		Title:       issue.Title,
		Description: desc,
		Done:        issue.IsClosed(),
	}
	if issue.IsClosed() && issue.ClosedAt != nil {
		task.DoneAt = *issue.ClosedAt
	}
	if issue.Milestone != nil && issue.Milestone.DueOn != nil {
		task.DueDate = *issue.Milestone.DueOn
	}

	if err := task.Create(s, actor); err != nil {
		return 0, err
	}

	if err := attachLabels(s, actor, task.ID, labelIDs); err != nil {
		return 0, err
	}
	return task.ID, nil
}

func updateMirroredTask(s *xorm.Session, actor *user.User, link *models.GitHubTaskLink, issue *Issue, labelIDs []int64) error {
	desc, err := richtext.MarkdownToHTML(buildDescription(link, issue))
	if err != nil {
		return fmt.Errorf("converting description: %w", err)
	}

	// Map-based update so reopening an item can null done_at/due_date — a
	// struct update would skip zero values and leave stale timestamps.
	updates := map[string]any{
		"title":       issue.Title,
		"description": desc,
		"done":        issue.IsClosed(),
		"done_at":     nil,
		"due_date":    nil,
	}
	if issue.IsClosed() && issue.ClosedAt != nil {
		updates["done_at"] = *issue.ClosedAt
	}
	if issue.Milestone != nil && issue.Milestone.DueOn != nil {
		updates["due_date"] = *issue.Milestone.DueOn
	}

	if err := models.ApplyMirroredTaskUpdate(s, actor, link.TaskID, updates); err != nil {
		return err
	}

	if err := attachLabels(s, actor, link.TaskID, labelIDs); err != nil {
		return err
	}
	return detachStaleLabels(s, actor, link.TaskID, labelIDs)
}

// linkRepoName adapts a link to the GitHubConnection shape buildDescription
// expects — links keep their own (current) owner/name.
type repoNamer interface {
	RepoFullName() string
}

// ensureLabels finds or creates the labels a GitHub item maps to and returns
// their ids: the repo label, the issue/PR kind label and one label per GitHub
// label (prefixed "gh:"). Label colors follow GitHub's.
func ensureLabels(s *xorm.Session, actor *user.User, conn *models.GitHubConnection, issue *Issue) ([]int64, error) {
	titles := []struct {
		title string
		color string
	}{
		{repoLabelPrefix + conn.RepoFullName(), ""},
		{issueLabelTitle, ""},
	}
	if issue.IsPullRequest() {
		titles[1] = struct{ title, color string }{pullRequestLabelTitle, ""}
	}
	for _, l := range issue.Labels {
		titles = append(titles, struct{ title, color string }{"gh:" + l.Name, l.Color})
	}

	ids := make([]int64, 0, len(titles))
	for _, want := range titles {
		label, err := ensureLabel(s, actor, want.title, want.color)
		if err != nil {
			return nil, err
		}
		ids = append(ids, label.ID)
	}
	return ids, nil
}

// ensureLabel returns the label with the exact title, creating it for the bot
// when missing. If it exists with a different color, the color is updated —
// GitHub label colors are authoritative for gh-prefixed labels.
func ensureLabel(s *xorm.Session, actor *user.User, title, color string) (*models.Label, error) {
	label := &models.Label{}
	has, err := s.Where("title = ?", title).Get(label)
	if err != nil {
		return nil, err
	}
	if has {
		normalized := utils.NormalizeHex(color)
		if normalized != "" && label.HexColor != normalized {
			label.HexColor = normalized
			if _, err := s.ID(label.ID).Cols("hex_color").Update(label); err != nil {
				return nil, err
			}
		}
		return label, nil
	}

	label = &models.Label{Title: title, HexColor: utils.NormalizeHex(color)}
	if err := label.Create(s, actor); err != nil {
		return nil, err
	}
	return label, nil
}

func attachLabels(s *xorm.Session, actor *user.User, taskID int64, labelIDs []int64) error {
	for _, id := range labelIDs {
		lt := &models.LabelTask{LabelID: id, TaskID: taskID}
		if err := lt.Create(s, actor); err != nil && !models.IsErrLabelIsAlreadyOnTask(err) {
			return err
		}
	}
	return nil
}

// detachStaleLabels removes labels managed by the sync (gh:/repo: prefixed)
// that are no longer on the GitHub item. User-added labels are never touched.
func detachStaleLabels(s *xorm.Session, actor *user.User, taskID int64, keep []int64) error {
	// label_task has no title column — resolve the managed ones via a join
	// through labels.
	results := []models.LabelTask{}
	err := s.
		Table("label_tasks").
		Join("INNER", "labels", "labels.id = label_tasks.label_id").
		Where("label_tasks.task_id = ?", taskID).
		And("(labels.title LIKE 'gh:%' OR labels.title LIKE 'repo:%')").
		Find(&results)
	if err != nil {
		return err
	}

	keepSet := make(map[int64]bool, len(keep))
	for _, id := range keep {
		keepSet[id] = true
	}
	for _, row := range results {
		if keepSet[row.LabelID] {
			continue
		}
		lt := &models.LabelTask{LabelID: row.LabelID, TaskID: taskID}
		if err := lt.Delete(s, actor); err != nil {
			return err
		}
	}
	return nil
}

// closingRefsRe matches "fixes #123", "closes #42", "resolves #7" (case
// insensitive) in PR bodies — the same keywords GitHub honors for auto-closing.
var closingRefsRe = regexp.MustCompile(`(?i)\b(?:fix(?:e[sd])?|close[sd]?|resolve[sd]?)\s+#(\d+)`)

// syncPullRequestRelations links a PR task to the tasks of issues its body
// says it fixes/closes/resolves. Only issues mirrored into the same project
// get a relation — GitHub items of other repos have no task to relate to.
func syncPullRequestRelations(s *xorm.Session, actor *user.User, _ *Client, conn *models.GitHubConnection, issue *Issue, taskID int64) error {
	if !issue.IsPullRequest() || issue.Body == "" {
		return nil
	}

	seen := make(map[int64]bool)
	for _, match := range closingRefsRe.FindAllStringSubmatch(issue.Body, -1) {
		number, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil || seen[number] {
			continue
		}
		seen[number] = true

		issueLink, err := getLinkByRepoNumber(s, conn, number)
		if err != nil || issueLink == nil {
			continue // issue not mirrored (other repo or filtered) — skip
		}
		if issueLink.TaskID == taskID {
			continue
		}

		rel := &models.TaskRelation{
			TaskID:       taskID,
			OtherTaskID:  issueLink.TaskID,
			RelationKind: models.RelationKindRelated,
		}
		if err := rel.Create(s, actor); err != nil && !models.IsErrRelationAlreadyExists(err) {
			return err
		}
	}
	return nil
}

func getLinkByRepoNumber(s *xorm.Session, conn *models.GitHubConnection, number int64) (*models.GitHubTaskLink, error) {
	link := &models.GitHubTaskLink{}
	has, err := s.
		Where("repo_owner = ? AND repo_name = ? AND number = ?", conn.RepoOwner, conn.RepoName, number).
		Get(link)
	if err != nil || !has {
		return nil, err
	}
	return link, nil
}

// contentHash fingerprints everything the mirror maps onto task fields, so
// unchanged items skip writes and the write-back phase can later tell user
// edits from its own syncs apart.
func contentHash(issue *Issue) string {
	labels := make([]string, 0, len(issue.Labels))
	for _, l := range issue.Labels {
		labels = append(labels, l.Name+":"+l.Color)
	}
	sort.Strings(labels)

	var dueOn, closedAt, mergedAt string
	if issue.Milestone != nil && issue.Milestone.DueOn != nil {
		dueOn = issue.Milestone.DueOn.UTC().Format(time.RFC3339)
	}
	if issue.ClosedAt != nil {
		closedAt = issue.ClosedAt.UTC().Format(time.RFC3339)
	}
	if issue.IsPullRequest() && issue.PullRequest.MergedAt != nil {
		mergedAt = issue.PullRequest.MergedAt.UTC().Format(time.RFC3339)
	}

	parts := []string{
		issue.NodeID,
		issue.Title,
		issue.Body,
		issue.State,
		dueOn,
		closedAt,
		mergedAt,
		strings.Join(labels, ","),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

// RecordSyncError persists a sync failure on the connection so the settings UI
// can surface it. Opens its own session: callers usually roll theirs back on
// failure, which would discard an error recorded on the same session.
func RecordSyncError(conn *models.GitHubConnection, syncErr error) {
	s := db.NewSession()
	defer func() {
		_ = s.Close()
	}()
	if _, err := s.ID(conn.ID).
		Cols("last_sync_error").
		Update(&models.GitHubConnection{LastSyncError: syncErr.Error()}); err != nil {
		log.Errorf("github: recording sync error for connection %d: %s (original error: %s)", conn.ID, err, syncErr)
		return
	}
	if err := s.Commit(); err != nil {
		log.Errorf("github: committing sync error for connection %d: %s", conn.ID, err)
	}
}
