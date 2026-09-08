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

	"xorm.io/builder"
	"xorm.io/xorm"
)

// GetProjectAudience returns the ids of all users who can see the given
// project: the owner, direct members, and members of teams the project is
// shared with. Used to fan out live task updates over websockets.
func GetProjectAudience(s *xorm.Session, projectID int64) ([]int64, error) {
	results := []struct {
		OwnerID int64 `xorm:"owner_id"`
	}{}
	if err := s.Table("projects").Select("owner_id").Where("id = ?", projectID).Find(&results); err != nil {
		return nil, err
	}

	members := []struct {
		UserID int64 `xorm:"user_id"`
	}{}
	if err := s.Table("users_projects").Select("user_id").Where("project_id = ?", projectID).Find(&members); err != nil {
		return nil, err
	}

	teamMembers := []struct {
		UserID int64 `xorm:"user_id"`
	}{}
	if err := s.Table("team_members").
		Select("team_members.user_id").
		Join("INNER", "team_projects", "team_projects.team_id = team_members.team_id").
		Where("team_projects.project_id = ?", projectID).
		Find(&teamMembers); err != nil {
		return nil, err
	}

	seen := make(map[int64]struct{}, len(results)+len(members)+len(teamMembers))
	userIDs := make([]int64, 0, len(results)+len(members)+len(teamMembers))
	add := func(id int64) {
		if id == 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		userIDs = append(userIDs, id)
	}
	for _, r := range results {
		add(r.OwnerID)
	}
	for _, m := range members {
		add(m.UserID)
	}
	for _, m := range teamMembers {
		add(m.UserID)
	}
	return userIDs, nil
}

// AddBucketsToTasks is the exported form of addBucketsToTasks for listeners
// that need to enrich tasks before pushing them to websocket clients.
func AddBucketsToTasks(s *xorm.Session, a web.Auth, taskIDs []int64, taskMap map[int64]*Task) error {
	return addBucketsToTasks(s, a, taskIDs, taskMap)
}

// AccessibleProjectsCond restricts a query to projects the auth can see
// (owner, membership, or team).
func AccessibleProjectsCond(a web.Auth, column string) builder.Cond {
	return accessibleProjectIDsSubquery(a, column)
}

// GetProjectViews returns all views of a project, ordered by position.
func GetProjectViews(s *xorm.Session, projectID int64) ([]*ProjectView, error) {
	return getViewsForProject(s, projectID)
}
