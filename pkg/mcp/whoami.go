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

package mcp

import (
	"context"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type whoamiOut struct {
	User        userRef               `json:"user"`
	IsBot       bool                  `json:"is_bot"`
	Permissions models.APIPermissions `json:"permissions"`
	Projects    []projectInfo         `json:"projects"`
}

type projectInfo struct {
	ID         int64             `json:"id"`
	Title      string            `json:"title"`
	Permission models.Permission `json:"permission" doc:"Your permission on this project. 0 = read, 1 = read/write, 2 = admin."`
}

// registerWhoamiTool is always registered for any token with mcp access — it
// is the agent's orientation call: identity, scopes and reachable projects.
func registerWhoamiTool(srv *mcp.Server, a *Auth) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "whoami",
		Description: "Returns who you are (the agent identity), your token's API permissions, and the projects you have access to with your permission level in each. Call this first to orient yourself.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, whoamiOut, error) {
		s := db.NewSession()
		defer s.Close()

		out := whoamiOut{
			User:        userRef{ID: a.User.ID, Username: a.User.Username, Name: a.User.Name},
			IsBot:       a.User.IsBot(),
			Permissions: a.Token.APIPermissions,
			Projects:    []projectInfo{},
		}

		memberships := []*models.ProjectUser{}
		if err := s.Where("user_id = ?", a.User.ID).Find(&memberships); err != nil {
			return nil, out, toolError("could not load memberships", err)
		}
		titles := map[int64]string{}
		if len(memberships) > 0 {
			ids := make([]int64, 0, len(memberships))
			for _, m := range memberships {
				ids = append(ids, m.ProjectID)
			}
			projects := []*models.Project{}
			if err := s.In("id", ids).Find(&projects); err != nil {
				return nil, out, toolError("could not load projects", err)
			}
			for _, p := range projects {
				titles[p.ID] = p.Title
			}
		}
		for _, m := range memberships {
			out.Projects = append(out.Projects, projectInfo{
				ID:         m.ProjectID,
				Title:      titles[m.ProjectID],
				Permission: m.Permission,
			})
		}
		return nil, out, nil
	})
}
