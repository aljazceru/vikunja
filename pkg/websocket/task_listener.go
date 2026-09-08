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

package websocket

import (
	"encoding/json"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/log"
	"code.vikunja.io/api/pkg/models"

	"github.com/ThreeDotsLabs/watermill/message"
)

// TaskListener pushes task changes to WebSocket clients so open boards update
// live — the piece that makes agent activity visible as it happens.
type TaskListener struct {
	wsEvent string
}

// Name returns the listener name.
func (l *TaskListener) Name() string {
	return "websocket.push." + l.wsEvent
}

// Handle processes a task event and pushes it to every user with access to the
// task's project. Recipients are resolved server-side; clients cannot subscribe
// to projects they cannot see.
func (l *TaskListener) Handle(msg *message.Message) error {
	var event models.TaskUpdatedEvent
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		return err
	}
	if event.Task == nil {
		return nil
	}

	s := db.NewSession()
	defer s.Close()

	task := event.Task
	taskMap := map[int64]*models.Task{task.ID: task}
	// Attach the task's buckets so Kanban clients can move the card to the
	// right column of the currently open view.
	if err := models.AddBucketsToTasks(s, event.Doer, []int64{task.ID}, taskMap); err != nil {
		log.Debugf("Could not enrich task %d with buckets for websocket push: %s", task.ID, err)
	}

	userIDs, err := models.GetProjectAudience(s, task.ProjectID)
	if err != nil {
		return err
	}

	hub := GetHub()
	for _, uid := range userIDs {
		hub.PublishForUser(uid, l.wsEvent, task)
	}
	return nil
}

// TaskDeletedListener tells open clients to drop a task from their views.
type TaskDeletedListener struct{}

// Name returns the listener name.
func (l *TaskDeletedListener) Name() string {
	return "websocket.push.task.deleted"
}

// Handle processes a task deleted event.
func (l *TaskDeletedListener) Handle(msg *message.Message) error {
	var event models.TaskDeletedEvent
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		return err
	}
	if event.Task == nil {
		return nil
	}

	s := db.NewSession()
	defer s.Close()

	userIDs, err := models.GetProjectAudience(s, event.Task.ProjectID)
	if err != nil {
		return err
	}

	hub := GetHub()
	for _, uid := range userIDs {
		hub.PublishForUser(uid, "task.deleted", event.Task)
	}
	return nil
}

func init() {
	events.RegisterListener((&models.TaskUpdatedEvent{}).Name(), &TaskListener{wsEvent: "task.updated"})
	events.RegisterListener((&models.TaskDeletedEvent{}).Name(), &TaskDeletedListener{})
}
