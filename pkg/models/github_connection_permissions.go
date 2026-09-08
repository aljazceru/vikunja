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
	"code.vikunja.io/api/pkg/web"

	"xorm.io/xorm"
)

func (c *GitHubConnection) CanRead(s *xorm.Session, a web.Auth) (bool, int, error) {
	// Reload the connection when only the id is set so the permission check
	// resolves the right project.
	if c.ProjectID == 0 && c.ID > 0 {
		existing := &GitHubConnection{}
		has, err := s.ID(c.ID).Get(existing)
		if err != nil {
			return false, 0, err
		}
		if !has {
			return false, 0, nil
		}
		c.ProjectID = existing.ProjectID
	}

	p := &Project{ID: c.ProjectID}
	return p.CanRead(s, a)
}

func (c *GitHubConnection) CanCreate(s *xorm.Session, a web.Auth) (bool, error) {
	return c.canDoConnection(s, a)
}

func (c *GitHubConnection) CanUpdate(s *xorm.Session, a web.Auth) (bool, error) {
	return c.canDoConnection(s, a)
}

func (c *GitHubConnection) CanDelete(s *xorm.Session, a web.Auth) (bool, error) {
	return c.canDoConnection(s, a)
}

// canDoConnection requires write access to the parent project — the same bar
// the project webhooks use. Managing a connection means creating tasks in the
// project, so read access alone is not enough.
func (c *GitHubConnection) canDoConnection(s *xorm.Session, a web.Auth) (bool, error) {
	if _, isShareAuth := a.(*LinkSharing); isShareAuth {
		return false, nil
	}

	if c.ID > 0 {
		existing := &GitHubConnection{}
		has, err := s.ID(c.ID).Get(existing)
		if err != nil {
			return false, err
		}
		if !has {
			return false, nil
		}
		c.ProjectID = existing.ProjectID
	}

	p := &Project{ID: c.ProjectID}
	return p.CanUpdate(s, a)
}
