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

package models

import (
	"time"

	"code.vikunja.io/api/pkg/web"

	"xorm.io/xorm"
)

// GitHubTaskLink is the bijection between a mirrored task and the GitHub item
// it represents. The GitHub node id is globally unique and stable across repo
// renames, which makes it the dedupe key: a re-created connection picking up a
// repo with existing links reuses the tasks instead of duplicating them.
//
// Links are internal — there is no HTTP CRUD for them; the sync engine in
// pkg/modules/github creates and updates them.
type GitHubTaskLink struct {
	ID     int64 `xorm:"bigint autoincr not null unique pk" json:"id"`
	TaskID int64 `xorm:"bigint not null unique(task) index" json:"task_id"`
	// Project the linked task lives in; denormalized so links can be found
	// per project without a join.
	ProjectID  int64 `xorm:"bigint not null index" json:"project_id"`
	Connection int64 `xorm:"bigint not null default 0 index 'connection_id'" json:"connection_id"`

	// "issue" or "pull_request".
	ItemType string `xorm:"varchar(20) not null" json:"item_type"`
	NodeID   string `xorm:"varchar(64) not null unique(node) index" json:"node_id"`
	Number   int64  `xorm:"bigint not null" json:"number"`

	RepoOwner string `xorm:"varchar(250) not null" json:"repo_owner"`
	RepoName  string `xorm:"varchar(250) not null" json:"repo_name"`
	HTMLURL   string `xorm:"varchar(500) null 'html_url'" json:"html_url"`

	// Hash of the mirrored content. Compared before writing to skip no-op
	// updates; the write-back phase will also use it to distinguish user
	// edits from its own sync writes.
	ContentHash string    `xorm:"varchar(64) null" json:"content_hash"`
	LastSeenAt  time.Time `xorm:"null" json:"last_seen_at"`

	Created time.Time `xorm:"created not null" json:"created"`
	Updated time.Time `xorm:"updated not null" json:"updated"`
}

// TableName returns the table name for GitHubTaskLink
func (*GitHubTaskLink) TableName() string {
	return "github_task_links"
}

// RepoFullName returns the repository in "owner/name" form.
func (l *GitHubTaskLink) RepoFullName() string {
	return l.RepoOwner + "/" + l.RepoName
}

// GetGitHubTaskLinkByNodeID resolves a GitHub item to its linked task, if any.
func GetGitHubTaskLinkByNodeID(s *xorm.Session, nodeID string) (*GitHubTaskLink, error) {
	link := &GitHubTaskLink{}
	has, err := s.Where("node_id = ?", nodeID).Get(link)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, nil
	}
	return link, nil
}

// ApplyMirroredTaskUpdate writes column updates to a task mirrored from an
// external system and runs the same "task was updated" side effects a regular
// task update triggers: the updated-timestamp bump (so delta syncs and CalDAV
// pick it up), the project bump, and the TaskUpdatedEvent (so notifications
// and subscriptions fire). Runs as the given doer, normally the sync bot.
func ApplyMirroredTaskUpdate(s *xorm.Session, doer web.Auth, taskID int64, updates map[string]any) error {
	// Where instead of ID: xorm's ID() needs a bean to resolve the table,
	// and map updates carry none.
	if _, err := s.Table("tasks").Where("id = ?", taskID).Update(updates); err != nil {
		return err
	}
	if err := updateTaskLastUpdated(s, &Task{ID: taskID, Updated: time.Now()}); err != nil {
		return err
	}
	if err := updateProjectByTaskID(s, taskID); err != nil {
		return err
	}
	return triggerTaskUpdatedEventForTaskID(s, doer, taskID)
}
