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
	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/user"

	"xorm.io/xorm"
)

// getSyncActor returns the user the sync runs as: whoever created the
// connection. Task creation resolves project access through the acting user
// (position recalculation reads the project with the task's creator), so a
// bot won't do — bots hold no project rights and there is deliberately no
// system bypass for them. This mirrors how the migration module creates tasks
// as the migrating user. Write-back (Phase 3) is where a bot actor with
// explicit grants gets revisited.
func getSyncActor(s *xorm.Session, conn *models.GitHubConnection) (*user.User, error) {
	u, err := user.GetUserByID(s, conn.CreatedByID)
	if err != nil {
		return nil, err
	}
	if u.Status == user.StatusDisabled {
		return nil, &user.ErrAccountDisabled{UserID: u.ID}
	}
	return u, nil
}
