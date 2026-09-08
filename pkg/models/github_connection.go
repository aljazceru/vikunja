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

	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"

	"xorm.io/xorm"
)

// GitHubConnection links a project to a GitHub repository. Issues and pull
// requests of that repository are mirrored as tasks into the project (several
// connections can target the same project; a per-repo label keeps them apart).
//
// Credentials are either a GitHub App installation (InstallationID > 0, tokens
// minted from the instance's app key) or a personal access token stored in
// Token.
type GitHubConnection struct {
	// The unique, numeric id of this connection.
	ID int64 `xorm:"bigint autoincr not null unique pk" json:"id" param:"connection" readOnly:"true" doc:"The unique, numeric id of this connection."`
	// The id of the project GitHub items are mirrored into. Set from the URL; ignored in the body.
	ProjectID int64 `xorm:"bigint not null index" json:"project_id" param:"project" readOnly:"true" doc:"The id of the project GitHub items are mirrored into. Set from the URL, not the body."`
	// The owner (user or org) of the GitHub repository.
	RepoOwner string `xorm:"varchar(250) not null" json:"repo_owner" valid:"required,runelength(1|250)" minLength:"1" maxLength:"250" doc:"The owner (user or organization) of the GitHub repository."`
	// The name of the GitHub repository, without the owner prefix.
	RepoName string `xorm:"varchar(250) not null" json:"repo_name" valid:"required,runelength(1|250)" minLength:"1" maxLength:"250" doc:"The name of the GitHub repository, without the owner prefix."`
	// The GitHub App installation authorizing access to the repo. 0 means the connection authenticates with a personal access token instead.
	InstallationID int64 `xorm:"bigint not null default 0" json:"installation_id" doc:"The id of the GitHub App installation that grants access to the repository. 0 when the connection uses a personal access token."`
	// A personal access token with read access to the repository. Write-only: never returned. Ignored when installation_id is set.
	Token string `xorm:"null" json:"token" writeOnly:"true" doc:"A GitHub personal access token with read (and repo-webhook) access to the repository. Write-only: never returned. Only used when installation_id is 0."`

	// Whether new GitHub items are mirrored and existing links kept up to date. Paused connections are skipped by syncs.
	Enabled bool `xorm:"not null default true" json:"enabled" doc:"Whether the connection actively syncs. Disabled connections are skipped by scheduled and manual syncs."`
	// When the connection last completed a sync.
	LastSyncedAt time.Time `xorm:"null" json:"last_synced_at" readOnly:"true" doc:"When the connection last completed a sync. Zero when it has never synced."`
	// The error of the last failed sync, if any. Cleared on the next successful sync.
	LastSyncError string `xorm:"null" json:"last_sync_error" readOnly:"true" doc:"The error message of the last failed sync, empty if the last sync succeeded."`

	WebhookSecret string `xorm:"null" json:"-"`
	RepoWebhookID int64  `xorm:"bigint not null default 0" json:"-"`

	CreatedByID int64 `xorm:"bigint not null" json:"-"`
	// The user who created the connection.
	CreatedBy *user.User `xorm:"-" json:"created_by" readOnly:"true" doc:"The user who created the connection."`
	// A timestamp when the connection was created. You cannot change this value.
	Created time.Time `xorm:"created not null" json:"created" readOnly:"true" doc:"A timestamp when this connection was created. You cannot change this value."`
	// A timestamp when the connection was last updated. You cannot change this value.
	Updated time.Time `xorm:"updated not null" json:"updated" readOnly:"true" doc:"A timestamp when this connection was last updated. You cannot change this value."`

	web.CRUDable    `xorm:"-" json:"-"`
	web.Permissions `xorm:"-" json:"-"`
}

// TableName returns the table name for GitHubConnection
func (*GitHubConnection) TableName() string {
	return "github_connections"
}

// RepoFullName returns the repository in "owner/name" form.
func (c *GitHubConnection) RepoFullName() string {
	return c.RepoOwner + "/" + c.RepoName
}

// maskCredentials clears the write-only token and the webhook secret so they
// are never echoed back in a response. The DB row keeps them; only the
// in-memory struct handed to the caller is cleared. Call after the DB write.
func (c *GitHubConnection) maskCredentials() {
	c.Token = ""
	c.WebhookSecret = ""
}

func (c *GitHubConnection) Create(s *xorm.Session, a web.Auth) error {
	u, err := user.GetFromAuth(a)
	if err != nil {
		return err
	}

	c.ID = 0
	c.CreatedByID = u.ID
	c.CreatedBy = u

	_, err = s.Insert(c)
	if err != nil {
		return err
	}

	c.maskCredentials()
	return nil
}

func (c *GitHubConnection) ReadOne(s *xorm.Session, _ web.Auth) error {
	conn, err := GetGitHubConnectionByID(s, c.ID)
	if err != nil {
		return err
	}
	*c = *conn
	return nil
}

// GetGitHubConnectionByID returns a connection by id. Credentials are
// masked — use this wherever the struct may end up in a response.
func GetGitHubConnectionByID(s *xorm.Session, id int64) (*GitHubConnection, error) {
	c, err := getGitHubConnectionByID(s, id)
	if err != nil {
		return nil, err
	}
	c.maskCredentials()
	return c, nil
}

// GetGitHubConnectionByIDWithCredentials returns a connection with its token
// and webhook secret intact — exclusively for the sync engine, which must
// authenticate against GitHub with them (the backfill reloads its row this
// way; masking there would sync with an empty token).
func GetGitHubConnectionByIDWithCredentials(s *xorm.Session, id int64) (*GitHubConnection, error) {
	return getGitHubConnectionByID(s, id)
}

func getGitHubConnectionByID(s *xorm.Session, id int64) (*GitHubConnection, error) {
	c := &GitHubConnection{}
	has, err := s.ID(id).Get(c)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, ErrGitHubConnectionDoesNotExist{ID: id}
	}
	return c, nil
}

func (c *GitHubConnection) ReadAll(s *xorm.Session, a web.Auth, _ string, page int, perPage int) (result any, resultCount int, numberOfTotalItems int64, err error) {
	// The generic list pipeline doesn't run CanRead, so the project's read
	// permission is enforced here.
	p := &Project{ID: c.ProjectID}
	can, _, err := p.CanRead(s, a)
	if err != nil {
		return nil, 0, 0, err
	}
	if !can {
		return nil, 0, 0, ErrGenericForbidden{}
	}

	limit, start := getLimitFromPageIndex(page, perPage)
	q := s.Where("project_id = ?", c.ProjectID)
	if limit > 0 {
		q = q.Limit(limit, start)
	}
	conns := []*GitHubConnection{}
	total, err := q.FindAndCount(&conns)
	if err != nil {
		return nil, 0, 0, err
	}
	for _, conn := range conns {
		conn.maskCredentials()
	}
	return conns, len(conns), total, nil
}

// Update toggles the enabled flag. Everything else about a connection is
// immutable after creation — credentials and repo can only be changed by
// deleting and re-creating the connection.
func (c *GitHubConnection) Update(s *xorm.Session, a web.Auth) error {
	_, err := s.ID(c.ID).
		Cols("enabled").
		Update(&GitHubConnection{Enabled: c.Enabled})
	if err != nil {
		return err
	}
	return c.ReadOne(s, a)
}

func (c *GitHubConnection) Delete(s *xorm.Session, _ web.Auth) error {
	_, err := s.ID(c.ID).Delete(&GitHubConnection{})
	return err
}

// GetGitHubConnectionsForSync returns all enabled connections in id order so
// the sync scheduler can walk them deterministically.
func GetGitHubConnectionsForSync(s *xorm.Session) ([]*GitHubConnection, error) {
	conns := []*GitHubConnection{}
	err := s.Where("enabled = ?", true).OrderBy("id").Find(&conns)
	return conns, err
}
