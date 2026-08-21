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
	"testing"

	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/user"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTaskListener verifies that task updates fan out to every user who can
// see the task's project (owner, direct member, team member) — the server-side
// half of the live kanban board.
func TestTaskListener(t *testing.T) {
	InitHub()

	s := setupNotificationListenerTest(t)
	doer, err := user.GetUserByID(s, 2) // a user unrelated to project 1
	require.NoError(t, err)
	// The listener opens its own session; anything still inside this
	// transaction is invisible to it (and holds the sqlite write lock).
	require.NoError(t, s.Commit())

	t.Run("pushes task.updated to the project audience", func(t *testing.T) {
		// Fixture project 3: owner user3, shared with user1 and user2 — all
		// three must receive the push; user6 must not.
		connOwner := taskEventConn(3, "task.updated")
		connSharer1 := taskEventConn(1, "task.updated")
		connMember := taskEventConn(2, "task.updated")
		connStranger := taskEventConn(6, "task.updated")
		hub := GetHub()
		hub.Register(connOwner)
		hub.Register(connSharer1)
		hub.Register(connMember)
		hub.Register(connStranger)

		listener := &TaskListener{wsEvent: "task.updated"}
		err := listener.Handle(newTaskUpdatedMessage(t, 1, 3, doer))
		require.NoError(t, err)

		ownerMsg := <-connOwner.send
		assert.Equal(t, "task.updated", ownerMsg.Event)

		sharerMsg := <-connSharer1.send
		assert.Equal(t, "task.updated", sharerMsg.Event)

		memberMsg := <-connMember.send
		assert.Equal(t, "task.updated", memberMsg.Event)

		select {
		case <-connStranger.send:
			t.Fatal("user without project access must not receive the push")
		default:
		}
	})

	t.Run("pushes task.deleted", func(t *testing.T) {
		connOwner := taskEventConn(3, "task.deleted")
		GetHub().Register(connOwner)

		listener := &TaskDeletedListener{}
		err := listener.Handle(newTaskUpdatedMessage(t, 1, 3, doer))
		require.NoError(t, err)

		msg := <-connOwner.send
		assert.Equal(t, "task.deleted", msg.Event)
	})
}

// newTaskUpdatedMessage builds a watermill message payload for a task event,
// mirroring what events.DispatchOnCommit produces.
func newTaskUpdatedMessage(t *testing.T, taskID, projectID int64, doer *user.User) *message.Message {
	t.Helper()
	payload, err := json.Marshal(&models.TaskUpdatedEvent{
		Task: &models.Task{ID: taskID, ProjectID: projectID},
		Doer: doer,
	})
	require.NoError(t, err)
	return message.NewMessage("test", payload)
}
