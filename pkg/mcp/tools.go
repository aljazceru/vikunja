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
	"time"

	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/user"

	"xorm.io/builder"
)

// Lean DTOs: the MCP output schemas are generated from these structs, so they
// stay deliberately small compared to the full REST models.

type userRef struct {
	ID       int64  `json:"id" doc:"The user's numeric id."`
	Username string `json:"username" doc:"The user's username."`
	Name     string `json:"name,omitempty" doc:"The user's display name, if set."`
}

func userRefFrom(u *user.User) *userRef {
	if u == nil {
		return nil
	}
	return &userRef{ID: u.ID, Username: u.Username, Name: u.Name}
}

type labelRef struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	HexColor string `json:"hex_color,omitempty"`
}

type bucketRef struct {
	ID            int64  `json:"id"`
	Title         string `json:"title"`
	ProjectViewID int64  `json:"project_view_id"`
}

type taskSummary struct {
	ID          int64      `json:"id"`
	Title       string     `json:"title"`
	Done        bool       `json:"done"`
	DueDate     *time.Time `json:"due_date,omitempty"`
	Priority    int64      `json:"priority"`
	PercentDone float64    `json:"percent_done"`
	ProjectID   int64      `json:"project_id"`
	Assignees   []userRef  `json:"assignees"`
	Labels      []labelRef `json:"labels,omitempty"`
}

type taskCommentDTO struct {
	ID      int64     `json:"id"`
	Comment string    `json:"comment"`
	Author  *userRef  `json:"author"`
	Created time.Time `json:"created"`
}

type taskDetail struct {
	taskSummary
	Description string           `json:"description,omitempty" doc:"The task description. May contain HTML."`
	StartDate   *time.Time       `json:"start_date,omitempty"`
	EndDate     *time.Time       `json:"end_date,omitempty"`
	DoneAt      *time.Time       `json:"done_at,omitempty"`
	Buckets     []bucketRef      `json:"buckets,omitempty" doc:"The buckets this task is in, per project view."`
	Comments    []taskCommentDTO `json:"comments,omitempty"`
	Created     time.Time        `json:"created"`
	Updated     time.Time        `json:"updated"`
	CreatedBy   *userRef         `json:"created_by"`
}

func taskSummaryFrom(t *models.Task) taskSummary {
	summary := taskSummary{
		ID:          t.ID,
		Title:       t.Title,
		Done:        t.Done,
		Priority:    t.Priority,
		PercentDone: t.PercentDone,
		ProjectID:   t.ProjectID,
		Assignees:   []userRef{},
	}
	if !t.DueDate.IsZero() {
		due := t.DueDate
		summary.DueDate = &due
	}
	for _, a := range t.Assignees {
		if a != nil {
			summary.Assignees = append(summary.Assignees, *userRefFrom(a))
		}
	}
	for _, l := range t.Labels {
		summary.Labels = append(summary.Labels, labelRef{ID: l.ID, Title: l.Title, HexColor: l.HexColor})
	}
	return summary
}

func taskDetailFrom(t *models.Task, comments []*models.TaskComment) taskDetail {
	d := taskDetail{taskSummary: taskSummaryFrom(t)}
	d.Description = t.Description
	if !t.StartDate.IsZero() {
		start := t.StartDate
		d.StartDate = &start
	}
	if !t.EndDate.IsZero() {
		end := t.EndDate
		d.EndDate = &end
	}
	if !t.DoneAt.IsZero() {
		doneAt := t.DoneAt
		d.DoneAt = &doneAt
	}
	for _, b := range t.Buckets {
		d.Buckets = append(d.Buckets, bucketRef{
			ID:            b.ID,
			Title:         b.Title,
			ProjectViewID: b.ProjectViewID,
		})
	}
	for _, c := range comments {
		d.Comments = append(d.Comments, taskCommentDTO{
			ID:      c.ID,
			Comment: c.Comment,
			Author:  userRefFrom(c.Author),
			Created: c.Created,
		})
	}
	d.Created = t.Created
	d.Updated = t.Updated
	d.CreatedBy = userRefFrom(t.CreatedBy)
	return d
}

// accessibleProjectsCond wraps the models helper that restricts a query to
// projects the auth can see (owner, membership, or team).
func accessibleProjectsCond(a *Auth, column string) builder.Cond {
	return models.AccessibleProjectsCond(a.User, column)
}
