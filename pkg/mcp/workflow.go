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
	"regexp"
	"strings"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"xorm.io/xorm"
)

// inProgressBucketRegex matches bucket titles that signal active work. When an
// agent claims a task we move it to such a bucket if the project has one, so
// humans watching the board see immediately that someone is on it.
var inProgressBucketRegex = regexp.MustCompile(`(?i)(in[ -]?progress|in[ -]?arbeit|doing|wip|working|bearbeitung)`)

func registerWorkflowTools(srv *mcp.Server, a *Auth) {
	if tokenHasPermission(a.Token, "tasks", "update") {
		registerAssignToMe(srv, a)
		registerUnassignMe(srv, a)
		registerSetTaskInProgress(srv, a)
		registerCompleteTask(srv, a)
	}
	if tokenHasPermission(a.Token, "tasks_comments", "create") {
		registerAddComment(srv, a)
	}
}

// findInProgressBucket looks for a bucket named like "In Progress" across the
// kanban views of a project. Returns the bucket and its view, or nils.
func findInProgressBucket(s *xorm.Session, projectID int64) (*models.Bucket, *models.ProjectView) {
	views, err := models.GetProjectViews(s, projectID)
	if err != nil {
		return nil, nil
	}
	for _, view := range views {
		if view.ViewKind != models.ProjectViewKindKanban {
			continue
		}
		buckets := []*models.Bucket{}
		if err := s.Where("project_view_id = ?", view.ID).OrderBy("position asc").Find(&buckets); err != nil {
			continue
		}
		for _, bucket := range buckets {
			if view.DoneBucketID != 0 && bucket.ID == view.DoneBucketID {
				continue
			}
			if inProgressBucketRegex.MatchString(bucket.Title) {
				return bucket, view
			}
		}
	}
	return nil, nil
}

// moveTaskToBucket moves a task into a bucket via the regular TaskBucket flow
// (bucket limits, done-bucket syncing and events included).
func moveTaskToBucket(ctx context.Context, a *Auth, task *models.Task, bucket *models.Bucket, view *models.ProjectView) error {
	s := db.NewSession()
	defer s.Close()

	tb := &models.TaskBucket{
		TaskID:        task.ID,
		ProjectID:     task.ProjectID,
		ProjectViewID: view.ID,
		BucketID:      bucket.ID,
	}
	can, err := tb.CanUpdate(s, a.User)
	if err != nil {
		return toolError("could not move task", err)
	}
	if !can {
		return toolError("forbidden: you cannot edit this task", &models.ErrTaskDoesNotExist{ID: task.ID})
	}
	if err := tb.Update(s, a.User); err != nil {
		_ = s.Rollback()
		return toolError("could not move task", err)
	}
	if err := s.Commit(); err != nil {
		return toolError("could not move task", err)
	}
	// DispatchOnCommit only queues; the queue drains after commit.
	events.DispatchPending(ctx, s)
	return nil
}

func registerAssignToMe(srv *mcp.Server, a *Auth) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "assign_to_me",
		Description: "Claims a task: assigns it to you and, if the project has a bucket named like \"In Progress\", moves the task there so everyone sees it is being worked on. Call this the moment you start working on a task. Returns the updated task.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		TaskID int64 `json:"task_id" doc:"The numeric id of the task to claim."`
	}) (*mcp.CallToolResult, taskDetail, error) {
		s := db.NewSession()
		defer s.Close()

		task := &models.Task{ID: in.TaskID}
		if err := task.ReadOne(s, a.User); err != nil {
			return nil, taskDetail{}, toolError("could not load task", err)
		}
		can, err := task.CanUpdate(s, a.User)
		if err != nil {
			return nil, taskDetail{}, toolError("cannot claim task", err)
		}
		if !can {
			return nil, taskDetail{}, toolError("forbidden: you cannot edit this task", &models.ErrTaskDoesNotExist{ID: in.TaskID})
		}

		assignee := &models.TaskAssginee{TaskID: in.TaskID, UserID: a.User.ID}
		if err := assignee.Create(s, a.User); err != nil {
			// An already-assigned agent re-claiming is not an error worth failing on.
			if !models.IsErrUserAlreadyAssigned(err) {
				_ = s.Rollback()
				return nil, taskDetail{}, toolError("could not assign task", err)
			}
		}

		// Same session and single commit as the web handlers: dispatching
		// mid-handler lets async listeners race the next write (a hard lock on
		// single-connection sqlite).
		bucket, view := findInProgressBucket(s, task.ProjectID)
		if bucket != nil && view != nil {
			tb := &models.TaskBucket{
				TaskID:        task.ID,
				ProjectID:     task.ProjectID,
				ProjectViewID: view.ID,
				BucketID:      bucket.ID,
			}
			if err := tb.Update(s, a.User); err != nil {
				_ = s.Rollback()
				return nil, taskDetail{}, toolError("could not move task", err)
			}
		}

		if err := s.Commit(); err != nil {
			return nil, taskDetail{}, toolError("could not claim task", err)
		}
		events.DispatchPending(ctx, s)

		s2 := db.NewSession()
		defer s2.Close()
		updated, comments, err := loadTaskWithComments(s2, a, in.TaskID)
		if err != nil {
			return nil, taskDetail{}, err
		}
		return nil, taskDetailFrom(updated, comments), nil
	})
}

func registerUnassignMe(srv *mcp.Server, a *Auth) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "unassign_me",
		Description: "Releases a task back to the pool by removing you as assignee. Use when you cannot finish it; leave a comment explaining the state.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		TaskID int64 `json:"task_id" doc:"The numeric id of the task."`
	}) (*mcp.CallToolResult, taskSummary, error) {
		s := db.NewSession()
		defer s.Close()

		task := &models.Task{ID: in.TaskID}
		if err := task.ReadOne(s, a.User); err != nil {
			return nil, taskSummary{}, toolError("could not load task", err)
		}
		can, err := task.CanUpdate(s, a.User)
		if err != nil {
			return nil, taskSummary{}, toolError("cannot unassign", err)
		}
		if !can {
			return nil, taskSummary{}, toolError("forbidden: you cannot edit this task", &models.ErrTaskDoesNotExist{ID: in.TaskID})
		}

		_, err = s.
			Where("task_id = ? AND user_id = ?", in.TaskID, a.User.ID).
			Delete(&models.TaskAssginee{})
		if err != nil {
			_ = s.Rollback()
			return nil, taskSummary{}, toolError("could not unassign", err)
		}
		if err := s.Commit(); err != nil {
			return nil, taskSummary{}, toolError("could not unassign", err)
		}
		events.DispatchPending(ctx, s)
		return nil, taskSummaryFrom(task), nil
	})
}

func registerSetTaskInProgress(srv *mcp.Server, a *Auth) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "set_task_in_progress",
		Description: "Moves a task to the project's \"In Progress\" bucket (or an explicit bucket), making active work visible on the kanban board. Prefer assign_to_me when you also want the task assigned to you.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		TaskID   int64 `json:"task_id" doc:"The numeric id of the task."`
		BucketID int64 `json:"bucket_id,omitempty" doc:"Explicit bucket id. If omitted, a bucket titled like \"In Progress\" is used when the project has one."`
	}) (*mcp.CallToolResult, taskSummary, error) {
		s := db.NewSession()
		defer s.Close()

		task := &models.Task{ID: in.TaskID}
		if err := task.ReadOne(s, a.User); err != nil {
			return nil, taskSummary{}, toolError("could not load task", err)
		}

		var bucket *models.Bucket
		var view *models.ProjectView
		if in.BucketID != 0 {
			buckets := []*models.Bucket{}
			if err := s.In("id", []int64{in.BucketID}).Find(&buckets); err != nil || len(buckets) == 0 {
				return nil, taskSummary{}, toolError("bucket not found", &models.ErrBucketDoesNotExist{BucketID: in.BucketID})
			}
			bucket = buckets[0]
			views, err := models.GetProjectViews(s, task.ProjectID)
			if err != nil {
				return nil, taskSummary{}, toolError("could not load views", err)
			}
			for _, v := range views {
				if v.ID == bucket.ProjectViewID {
					view = v
					break
				}
			}
			if view == nil {
				return nil, taskSummary{}, toolError("bucket not found", &models.ErrBucketDoesNotExist{BucketID: in.BucketID})
			}
		} else {
			bucket, view = findInProgressBucket(s, task.ProjectID)
			if bucket == nil {
				return nil, taskSummary{}, toolError("no in-progress bucket",
					&models.ErrBucketDoesNotExist{BucketID: 0})
			}
		}

		if err := moveTaskToBucket(ctx, a, task, bucket, view); err != nil {
			return nil, taskSummary{}, err
		}

		s2 := db.NewSession()
		defer s2.Close()
		updated := &models.Task{ID: in.TaskID}
		if err := updated.ReadOne(s2, a.User); err != nil {
			return nil, taskSummary{}, toolError("could not reload task", err)
		}
		return nil, taskSummaryFrom(updated), nil
	})
}

func registerCompleteTask(srv *mcp.Server, a *Auth) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "complete_task",
		Description: "Marks a task done. If the project has a done bucket configured, the task lands there automatically. Post a final comment summarising the result before calling this.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		TaskID int64 `json:"task_id" doc:"The numeric id of the task."`
	}) (*mcp.CallToolResult, taskSummary, error) {
		s := db.NewSession()
		defer s.Close()

		task := &models.Task{ID: in.TaskID}
		if err := task.ReadOne(s, a.User); err != nil {
			return nil, taskSummary{}, toolError("could not load task", err)
		}
		can, err := task.CanUpdate(s, a.User)
		if err != nil {
			return nil, taskSummary{}, toolError("cannot complete task", err)
		}
		if !can {
			return nil, taskSummary{}, toolError("forbidden: you cannot edit this task", &models.ErrTaskDoesNotExist{ID: in.TaskID})
		}

		task.Done = true
		if err := task.Update(s, a.User); err != nil {
			_ = s.Rollback()
			return nil, taskSummary{}, toolError("could not complete task", err)
		}
		if err := s.Commit(); err != nil {
			return nil, taskSummary{}, toolError("could not complete task", err)
		}
		events.DispatchPending(ctx, s)
		return nil, taskSummaryFrom(task), nil
	})
}

func registerAddComment(srv *mcp.Server, a *Auth) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "add_comment",
		Description: "Adds a comment to a task — the way you report progress, findings and results to humans and other agents. Plain text; HTML is allowed but not required.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		TaskID  int64  `json:"task_id" doc:"The numeric id of the task."`
		Comment string `json:"comment" doc:"The comment text."`
	}) (*mcp.CallToolResult, taskCommentDTO, error) {
		s := db.NewSession()
		defer s.Close()

		tc := &models.TaskComment{
			TaskID:  in.TaskID,
			Comment: strings.TrimSpace(in.Comment),
		}
		if tc.Comment == "" {
			return nil, taskCommentDTO{}, toolError("comment must not be empty", nil)
		}

		can, err := tc.CanCreate(s, a.User)
		if err != nil {
			return nil, taskCommentDTO{}, toolError("cannot comment", err)
		}
		if !can {
			return nil, taskCommentDTO{}, toolError("forbidden: you cannot comment on this task", &models.ErrTaskDoesNotExist{ID: in.TaskID})
		}

		if err := tc.Create(s, a.User); err != nil {
			_ = s.Rollback()
			return nil, taskCommentDTO{}, toolError("could not add comment", err)
		}
		if err := s.Commit(); err != nil {
			return nil, taskCommentDTO{}, toolError("could not add comment", err)
		}
		events.DispatchPending(ctx, s)
		return nil, taskCommentDTO{
			ID:      tc.ID,
			Comment: tc.Comment,
			Author:  userRefFrom(tc.Author),
			Created: tc.Created,
		}, nil
	})
}
