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
	"time"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"xorm.io/builder"
	"xorm.io/xorm"
)

func registerTaskTools(srv *mcp.Server, a *Auth) {
	if tokenHasPermission(a.Token, "tasks", "read_all") {
		registerListTasks(srv, a)
	}
	if tokenHasPermission(a.Token, "tasks", "read_one") {
		registerGetTask(srv, a)
	}
	if tokenHasPermission(a.Token, "tasks", "create") {
		registerCreateTask(srv, a)
	}
	if tokenHasPermission(a.Token, "tasks", "update") {
		registerUpdateTask(srv, a)
	}
}

func registerListTasks(srv *mcp.Server, a *Auth) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_tasks",
		Description: "Lists tasks you can see, newest first. Filter by project, done state, assignee (use -1 for yourself, true for unassigned) or changes since a timestamp. This is how you find available work.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in struct {
		ProjectID    int64      `json:"project_id,omitempty" doc:"Restrict to one project."`
		Done         *bool      `json:"done,omitempty" doc:"Filter by done state. Omit for all."`
		AssigneeID   int64      `json:"assignee_id,omitempty" doc:"Filter by assignee user id; -1 means yourself. Use unassigned=true for tasks with no assignee."`
		Unassigned   *bool      `json:"unassigned,omitempty" doc:"Only tasks with no assignee at all."`
		UpdatedSince *time.Time `json:"updated_since,omitempty" doc:"Only tasks updated after this timestamp (RFC3339)."`
		Page         int        `json:"page,omitempty" minimum:"1" doc:"Page number, 1-based."`
		PerPage      int        `json:"per_page,omitempty" minimum:"1" maximum:"100" doc:"Page size, default 20."`
	}) (*mcp.CallToolResult, []taskSummary, error) {
		s := db.NewSession()
		defer s.Close()

		cond := builder.And(
			accessibleProjectsCond(a, "tasks.project_id"),
			builder.Eq{"tasks.is_archived": false},
		)
		if in.ProjectID != 0 {
			cond = cond.And(builder.Eq{"tasks.project_id": in.ProjectID})
		}
		if in.Done != nil {
			cond = cond.And(builder.Eq{"tasks.done": *in.Done})
		}
		if in.UpdatedSince != nil {
			cond = cond.And(builder.Gt{"tasks.updated": *in.UpdatedSince})
		}
		if in.AssigneeID != 0 {
			if in.AssigneeID < 0 {
				in.AssigneeID = a.User.ID
			}
			cond = cond.And(builder.In("tasks.id", builder.Select("task_id").
				From("task_assignees").Where(builder.Eq{"user_id": in.AssigneeID})))
		}
		if in.Unassigned != nil && *in.Unassigned {
			cond = cond.And(builder.NotIn("tasks.id", builder.Select("task_id").From("task_assignees")))
		}

		perPage := in.PerPage
		if perPage == 0 {
			perPage = 20
		}
		if perPage > 100 {
			perPage = 100
		}
		page := in.Page
		if page == 0 {
			page = 1
		}

		tasks := []*models.Task{}
		err := s.
			Where(cond).
			OrderBy("tasks.updated desc").
			Limit(perPage, (page-1)*perPage).
			Find(&tasks)
		if err != nil {
			return nil, nil, toolError("could not load tasks", err)
		}

		// Bulk-enrich assignees + labels for the result set.
		taskIDs := make([]int64, 0, len(tasks))
		taskMap := map[int64]*models.Task{}
		for _, t := range tasks {
			taskIDs = append(taskIDs, t.ID)
			taskMap[t.ID] = t
		}
		if err := models.AddAssigneesToTasks(s, taskIDs, taskMap); err != nil {
			return nil, nil, toolError("could not load assignees", err)
		}
		if err := models.AddLabelsToTasks(s, taskIDs, taskMap); err != nil {
			return nil, nil, toolError("could not load labels", err)
		}
		out := make([]taskSummary, 0, len(tasks))
		for _, t := range tasks {
			out = append(out, taskSummaryFrom(t))
		}
		return nil, out, nil
	})
}

func registerGetTask(srv *mcp.Server, a *Auth) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_task",
		Description: "Returns one task in full detail: description, dates, assignees, labels, kanban buckets and all comments. Always read a task before working on it.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in struct {
		ID int64 `json:"id" doc:"The numeric id of the task."`
	}) (*mcp.CallToolResult, taskDetail, error) {
		s := db.NewSession()
		defer s.Close()

		t, comments, err := loadTaskWithComments(s, a, in.ID)
		if err != nil {
			return nil, taskDetail{}, err
		}
		return nil, taskDetailFrom(t, comments), nil
	})
}

// loadTaskWithComments loads a task the auth can read, enriched with
// assignees, labels, buckets and comments.
func loadTaskWithComments(s *xorm.Session, a *Auth, taskID int64) (*models.Task, []*models.TaskComment, error) {
	t := &models.Task{ID: taskID}
	can, _, err := t.CanRead(s, a.User)
	if err != nil {
		return nil, nil, toolError("could not load task", err)
	}
	if !can {
		return nil, nil, toolError("forbidden", &models.ErrTaskDoesNotExist{ID: taskID})
	}
	if err := t.ReadOne(s, a.User); err != nil {
		return nil, nil, toolError("could not load task", err)
	}

	taskMap := map[int64]*models.Task{t.ID: t}
	_ = models.AddBucketsToTasks(s, a.User, []int64{t.ID}, taskMap)

	// Comments are best-effort detail; a failure to load them must not fail
	// the whole tool call.
	tc := &models.TaskComment{TaskID: t.ID}
	result, _, _, commentsErr := tc.ReadAll(s, a.User, "", 1, 100)
	var comments []*models.TaskComment
	if commentsErr == nil {
		comments, _ = result.([]*models.TaskComment)
	}
	return t, comments, nil
}

func registerCreateTask(srv *mcp.Server, a *Auth) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_task",
		Description: "Creates a task in a project you have write access to. Only create tasks when explicitly asked to; humans usually create the work and you pick it up.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ProjectID   int64      `json:"project_id" doc:"The numeric id of the project."`
		Title       string     `json:"title" doc:"The task title."`
		Description string     `json:"description,omitempty" doc:"The task description. May contain HTML."`
		DueDate     *time.Time `json:"due_date,omitempty" doc:"Due date, RFC3339."`
		Priority    int64      `json:"priority,omitempty" minimum:"0" maximum:"100" doc:"Priority 0-100, higher is more urgent."`
	}) (*mcp.CallToolResult, taskSummary, error) {
		s := db.NewSession()
		defer s.Close()

		t := &models.Task{
			ProjectID:   in.ProjectID,
			Title:       in.Title,
			Description: in.Description,
			Priority:    in.Priority,
		}
		if in.DueDate != nil {
			t.DueDate = *in.DueDate
		}

		can, err := t.CanCreate(s, a.User)
		if err != nil {
			return nil, taskSummary{}, toolError("could not create task", err)
		}
		if !can {
			return nil, taskSummary{}, toolError("forbidden", &models.ErrNeedToHaveProjectReadAccess{ProjectID: in.ProjectID})
		}
		if err := t.Create(s, a.User); err != nil {
			_ = s.Rollback()
			return nil, taskSummary{}, toolError("could not create task", err)
		}
		if err := s.Commit(); err != nil {
			return nil, taskSummary{}, toolError("could not create task", err)
		}
		events.DispatchPending(ctx, s)
		return nil, taskSummaryFrom(t), nil
	})
}

func registerUpdateTask(srv *mcp.Server, a *Auth) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "update_task",
		Description: "Updates a task's fields (title, description, due date, priority, percent done). Fields you omit are left unchanged. To change assignment or done state prefer assign_to_me / complete_task.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ID          int64      `json:"id" doc:"The numeric id of the task."`
		Title       *string    `json:"title,omitempty"`
		Description *string    `json:"description,omitempty" doc:"The task description. May contain HTML."`
		DueDate     *time.Time `json:"due_date,omitempty"`
		Priority    *int64     `json:"priority,omitempty" minimum:"0" maximum:"100"`
		PercentDone *float64   `json:"percent_done,omitempty" minimum:"0" maximum:"1"`
	}) (*mcp.CallToolResult, taskSummary, error) {
		s := db.NewSession()
		defer s.Close()

		t := &models.Task{ID: in.ID}
		if err := t.ReadOne(s, a.User); err != nil {
			return nil, taskSummary{}, toolError("could not load task", err)
		}
		can, err := t.CanUpdate(s, a.User)
		if err != nil {
			return nil, taskSummary{}, toolError("could not update task", err)
		}
		if !can {
			return nil, taskSummary{}, toolError("forbidden", &models.ErrTaskDoesNotExist{ID: in.ID})
		}

		if in.Title != nil {
			t.Title = *in.Title
		}
		if in.Description != nil {
			t.Description = *in.Description
		}
		if in.DueDate != nil {
			t.DueDate = *in.DueDate
		}
		if in.Priority != nil {
			t.Priority = *in.Priority
		}
		if in.PercentDone != nil {
			t.PercentDone = *in.PercentDone
		}

		if err := t.Update(s, a.User); err != nil {
			_ = s.Rollback()
			return nil, taskSummary{}, toolError("could not update task", err)
		}
		if err := s.Commit(); err != nil {
			return nil, taskSummary{}, toolError("could not update task", err)
		}
		events.DispatchPending(ctx, s)
		return nil, taskSummaryFrom(t), nil
	})
}
