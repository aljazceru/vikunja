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
	"strings"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type projectSummary struct {
	ID              int64  `json:"id"`
	Title           string `json:"title"`
	Description     string `json:"description,omitempty"`
	ParentProjectID *int64 `json:"parent_project_id,omitempty"`
	IsArchived      bool   `json:"is_archived"`
}

type viewInfo struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Kind  string `json:"kind"`
}

type projectDetail struct {
	projectSummary
	Views []viewInfo `json:"views"`
}

func registerProjectTools(srv *mcp.Server, a *Auth) {
	if tokenHasPermission(a.Token, "projects", "read_all") {
		mcp.AddTool(srv, &mcp.Tool{
			Name:        "list_projects",
			Description: "Lists all projects you have access to (owned, member of, or via teams), with id, title and archived state. Use the ids with list_tasks.",
		}, func(_ context.Context, _ *mcp.CallToolRequest, in struct {
			Search string `json:"search,omitempty" doc:"Filter by title substring."`
		}) (*mcp.CallToolResult, []projectSummary, error) {
			s := db.NewSession()
			defer s.Close()

			projects := []*models.Project{}
			err := s.
				Where(accessibleProjectsCond(a, "projects.id")).
				And("is_archived = false").
				Find(&projects)
			if err != nil {
				return nil, nil, toolError("could not load projects", err)
			}
			out := []projectSummary{}
			for _, p := range projects {
				if in.Search != "" && !strings.Contains(strings.ToLower(p.Title), strings.ToLower(in.Search)) {
					continue
				}
				out = append(out, projectSummary{
					ID:              p.ID,
					Title:           p.Title,
					Description:     p.Description,
					ParentProjectID: p.ParentProjectID,
					IsArchived:      p.IsArchived,
				})
			}
			return nil, out, nil
		})
	}

	if tokenHasPermission(a.Token, "projects", "read_one") {
		mcp.AddTool(srv, &mcp.Tool{
			Name:        "get_project",
			Description: "Returns one project by id, including its views (list, kanban, gantt...). Kanban view ids are needed for bucket operations.",
		}, func(_ context.Context, _ *mcp.CallToolRequest, in struct {
			ID int64 `json:"id" doc:"The numeric id of the project."`
		}) (*mcp.CallToolResult, projectDetail, error) {
			s := db.NewSession()
			defer s.Close()

			p := &models.Project{ID: in.ID}
			can, _, err := p.CanRead(s, a.User)
			if err != nil {
				return nil, projectDetail{}, toolError("could not load project", err)
			}
			if !can {
				return nil, projectDetail{}, toolError("forbidden", &models.ErrNeedToHaveProjectReadAccess{ProjectID: in.ID})
			}
			if err := p.ReadOne(s, a.User); err != nil {
				return nil, projectDetail{}, toolError("could not load project", err)
			}

			out := projectDetail{
				projectSummary: projectSummary{
					ID:              p.ID,
					Title:           p.Title,
					Description:     p.Description,
					ParentProjectID: p.ParentProjectID,
					IsArchived:      p.IsArchived,
				},
				Views: []viewInfo{},
			}
			views, err := models.GetProjectViews(s, p.ID)
			if err != nil {
				return nil, out, toolError("could not load views", err)
			}
			for _, v := range views {
				out.Views = append(out.Views, viewInfo{ID: v.ID, Title: v.Title, Kind: viewKindName(v.ViewKind)})
			}
			return nil, out, nil
		})
	}
}

func viewKindName(k models.ProjectViewKind) string {
	switch k {
	case models.ProjectViewKindList:
		return "list"
	case models.ProjectViewKindGantt:
		return "gantt"
	case models.ProjectViewKindTable:
		return "table"
	case models.ProjectViewKindKanban:
		return "kanban"
	default:
		return "unknown"
	}
}
