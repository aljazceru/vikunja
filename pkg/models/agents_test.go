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
	"testing"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedAgentRouteTables registers the route groups the agent presets grant,
// mirroring what collectRoutesForAPITokens does in a running server. Model
// tests don't boot the echo router, so the tables start empty.
func seedAgentRouteTables(t *testing.T) {
	t.Helper()
	for _, r := range []echo.RouteInfo{
		{Method: "GET", Path: "/api/v1/tasks"},
		{Method: "GET", Path: "/api/v1/tasks/:id"},
		{Method: "PUT", Path: "/api/v1/tasks"},
		{Method: "POST", Path: "/api/v1/tasks/:id"},
		{Method: "DELETE", Path: "/api/v1/tasks/:id"},
		{Method: "GET", Path: "/api/v1/tasks/:task/comments"},
		{Method: "PUT", Path: "/api/v1/tasks/:task/comments"},
		{Method: "GET", Path: "/api/v1/projects"},
		{Method: "GET", Path: "/api/v1/projects/:project"},
		{Method: "POST", Path: "/api/v1/projects/:project"},
		{Method: "GET", Path: "/api/v1/labels"},
		{Method: "PUT", Path: "/api/v1/labels"},
	} {
		CollectRoutesForAPITokenUsage(r, true)
	}
}

func TestCreateAgent(t *testing.T) {
	u := &user.User{ID: 1}
	s := db.NewSession()
	defer s.Close()
	db.LoadAndAssertFixtures(t)
	seedAgentRouteTables(t)

	t.Run("read-write preset", func(t *testing.T) {
		agent, token, err := CreateAgent(s, u, &AgentCreate{
			Name:   "Research Agent",
			Preset: AgentPresetReadWrite,
			Projects: []AgentProjectMembership{
				{ProjectID: 1, Permission: PermissionWrite},
			},
		})
		require.NoError(t, err)
		require.NoError(t, s.Commit())

		assert.NotZero(t, agent.ID)
		assert.Equal(t, "bot-research-agent", agent.Username)
		assert.Len(t, agent.Tokens, 1)
		assert.Contains(t, token, APITokenPrefix)

		perms := agent.Tokens[0].APIPermissions
		assert.Contains(t, perms["mcp"], "access")
		assert.Contains(t, perms["tasks"], "update")
		assert.Contains(t, perms["tasks_comments"], "create")

		assert.Len(t, agent.Projects, 1)
		assert.Equal(t, int64(1), agent.Projects[0].ProjectID)
		assert.Equal(t, PermissionWrite, agent.Projects[0].Permission)

		// The bot user must exist and be owned by the caller.
		bot, err := user.GetUserByID(s, agent.ID)
		require.NoError(t, err)
		assert.True(t, bot.IsBot())
		assert.Equal(t, u.ID, bot.BotOwnerID)

		// Token must authenticate as the bot.
		validated, owner, err := ValidateTokenAndGetOwner(s, token)
		require.NoError(t, err)
		require.NotNil(t, validated)
		assert.Equal(t, agent.ID, owner.ID)
		assert.True(t, validated.HasMCPAccess())
	})

	t.Run("read-only preset cannot write tasks", func(t *testing.T) {
		agent, _, err := CreateAgent(s, u, &AgentCreate{
			Name:     "Watcher",
			Preset:   AgentPresetReadOnly,
			Projects: nil,
		})
		require.NoError(t, err)
		require.NoError(t, s.Commit())

		perms := agent.Tokens[0].APIPermissions
		assert.NotContains(t, perms["tasks"], "update")
		assert.NotContains(t, perms["tasks"], "create")
		assert.NotContains(t, perms["tasks_comments"], "create")
	})

	t.Run("invalid preset is rejected", func(t *testing.T) {
		_, _, err := CreateAgent(s, u, &AgentCreate{Name: "x", Preset: "yolo"})
		require.Error(t, err)
		var invalid *ErrInvalidAgentPreset
		require.ErrorAs(t, err, &invalid)
		_ = s.Rollback()
	})

	t.Run("non-admin project is rejected", func(t *testing.T) {
		// Project 5 belongs to user2; user1 has no admin rights there.
		_, _, err := CreateAgent(s, u, &AgentCreate{
			Name:     "sneaky",
			Preset:   AgentPresetReadOnly,
			Projects: []AgentProjectMembership{{ProjectID: 5, Permission: PermissionRead}},
		})
		require.Error(t, err)
		_ = s.Rollback()
	})

	t.Run("bots cannot create agents", func(t *testing.T) {
		bot, err := user.GetUserByID(s, 23) // fixture bot owned by user 21
		require.NoError(t, err)
		require.True(t, bot.IsBot())
		_, _, err = CreateAgent(s, bot, &AgentCreate{Name: "bot child", Preset: AgentPresetReadOnly})
		require.Error(t, err)
		_ = s.Rollback()
	})
}

func TestAgentScoping(t *testing.T) {
	u := &user.User{ID: 1}
	s := db.NewSession()
	defer s.Close()
	db.LoadAndAssertFixtures(t)
	seedAgentRouteTables(t)

	// Project 1 is owned by user1; project 5 belongs to user2.
	agent, token, err := CreateAgent(s, u, &AgentCreate{
		Name:     "scoped",
		Preset:   AgentPresetReadWrite,
		Projects: []AgentProjectMembership{{ProjectID: 1, Permission: PermissionWrite}},
	})
	require.NoError(t, err)
	require.NoError(t, s.Commit())

	_, owner, err := ValidateTokenAndGetOwner(s, token)
	require.NoError(t, err)

	t.Run("member project is readable", func(t *testing.T) {
		p := &Project{ID: 1}
		can, _, err := p.CanRead(s, owner)
		require.NoError(t, err)
		assert.True(t, can)
	})

	t.Run("non-member project is denied", func(t *testing.T) {
		p := &Project{ID: 5}
		can, _, err := p.CanRead(s, owner)
		require.NoError(t, err)
		assert.False(t, can)
	})

	t.Run("membership shows in whoami-style listing", func(t *testing.T) {
		fetched, err := GetAgent(s, u, agent.ID)
		require.NoError(t, err)
		require.Len(t, fetched.Projects, 1)
		assert.Equal(t, int64(1), fetched.Projects[0].ProjectID)
	})
}

func TestRotateAndDeleteAgent(t *testing.T) {
	u := &user.User{ID: 1}
	s := db.NewSession()
	defer s.Close()
	db.LoadAndAssertFixtures(t)
	seedAgentRouteTables(t)

	agent, oldToken, err := CreateAgent(s, u, &AgentCreate{
		Name:     "rotate me",
		Preset:   AgentPresetComment,
		Projects: []AgentProjectMembership{{ProjectID: 1, Permission: PermissionWrite}},
	})
	require.NoError(t, err)
	require.NoError(t, s.Commit())

	rotated, newToken, err := RotateAgentToken(s, u, agent.ID, "")
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	assert.NotEqual(t, oldToken, newToken)
	assert.Len(t, rotated.Tokens, 1)

	// Old token is revoked.
	revoked, _, err := ValidateTokenAndGetOwner(s, oldToken)
	require.NoError(t, err)
	assert.Nil(t, revoked, "old token must be invalid after rotation")

	// New token works and keeps the preset.
	validated, owner, err := ValidateTokenAndGetOwner(s, newToken)
	require.NoError(t, err)
	require.NotNil(t, validated)
	assert.Equal(t, agent.ID, owner.ID)

	// A different user cannot manage the agent.
	other := &user.User{ID: 2}
	_, _, err = RotateAgentToken(s, other, agent.ID, "")
	require.Error(t, err)
	assert.True(t, IsErrGenericForbidden(err))
	_ = s.Rollback()

	require.NoError(t, s.Commit())

	// Delete cascades token + membership.
	err = DeleteAgent(s, u, agent.ID)
	require.NoError(t, err)
	require.NoError(t, s.Commit())

	_, err = user.GetUserByID(s, agent.ID)
	require.Error(t, err)
	deleted, _, err := ValidateTokenAndGetOwner(s, newToken)
	require.NoError(t, err)
	assert.Nil(t, deleted, "token must be gone with the agent")

	memberships := []*ProjectUser{}
	err = s.Where("user_id = ?", agent.ID).Find(&memberships)
	require.NoError(t, err)
	assert.Empty(t, memberships)
}

func TestAgentLastUsed(t *testing.T) {
	u := &user.User{ID: 1}
	s := db.NewSession()
	defer s.Close()
	db.LoadAndAssertFixtures(t)

	agent, token, err := CreateAgent(s, u, &AgentCreate{Name: "hb", Preset: AgentPresetReadOnly})
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	require.NoError(t, agent.Tokens[0].TouchLastUsed(s))
	require.NoError(t, s.Commit())

	listed, err := GetAgent(s, u, agent.ID)
	require.NoError(t, err)
	assert.False(t, listed.LastUsedAt.IsZero())

	_ = token
}
