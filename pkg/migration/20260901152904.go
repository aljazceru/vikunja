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

type githubConnection20260901152904 struct {
	ID             int64     `xorm:"bigint autoincr not null unique pk"`
	ProjectID      int64     `xorm:"bigint not null index"`
	RepoOwner      string    `xorm:"varchar(250) not null"`
	RepoName       string    `xorm:"varchar(250) not null"`
	InstallationID int64     `xorm:"bigint not null default 0"`
	Token          string    `xorm:"null"`
	Enabled        bool      `xorm:"not null default true"`
	LastSyncedAt   time.Time `xorm:"null"`
	LastSyncError  string    `xorm:"null"`
	WebhookSecret  string    `xorm:"null"`
	RepoWebhookID  int64     `xorm:"bigint not null default 0"`
	CreatedByID    int64     `xorm:"bigint not null"`
	Created        time.Time `xorm:"created not null"`
	Updated        time.Time `xorm:"updated not null"`
}

func (githubConnection20260901152904) TableName() string {
	return "github_connections"
}

func init() {
	migrations = append(migrations, &xormigrate.Migration{
		ID:          "20260901152904",
		Description: "Create github_connections table for the GitHub integration",
		Migrate: func(tx *xorm.Engine) error {
			return tx.Sync2(githubConnection20260901152904{}) //nolint:forbidigo // brand-new table, nothing to drop
		},
		Rollback: func(tx *xorm.Engine) error {
			return nil
		},
	})
}
