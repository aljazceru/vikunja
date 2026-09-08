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

package migration

import (
	"time"

	"src.techknowlogick.com/xormigrate"
	"xorm.io/xorm"
)

type githubTaskLink20260901152905 struct {
	ID          int64     `xorm:"bigint autoincr not null unique pk"`
	TaskID      int64     `xorm:"bigint not null unique(task) index"`
	ProjectID   int64     `xorm:"bigint not null index"`
	Connection  int64     `xorm:"bigint not null default 0 index 'connection_id'"`
	ItemType    string    `xorm:"varchar(20) not null"`
	NodeID      string    `xorm:"varchar(64) not null unique(node) index"`
	Number      int64     `xorm:"bigint not null"`
	RepoOwner   string    `xorm:"varchar(250) not null"`
	RepoName    string    `xorm:"varchar(250) not null"`
	HTMLURL     string    `xorm:"varchar(500) null 'html_url'"`
	ContentHash string    `xorm:"varchar(64) null"`
	LastSeenAt  time.Time `xorm:"null"`
	Created     time.Time `xorm:"created not null"`
	Updated     time.Time `xorm:"updated not null"`
}

func (githubTaskLink20260901152905) TableName() string {
	return "github_task_links"
}

func init() {
	migrations = append(migrations, &xormigrate.Migration{
		ID:          "20260901152905",
		Description: "Create github_task_links table for the GitHub integration",
		Migrate: func(tx *xorm.Engine) error {
			return tx.Sync2(githubTaskLink20260901152905{}) //nolint:forbidigo // brand-new table, nothing to drop
		},
		Rollback: func(tx *xorm.Engine) error {
			return nil
		},
	})
}
