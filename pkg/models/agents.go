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
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/utils"
	"code.vikunja.io/api/pkg/web"

	"xorm.io/xorm"
)

// Agent presets: named bundles of API-token route permissions. They restrict
// *what* an agent can do; project memberships restrict *where*.
const (
	AgentPresetReadOnly  = "read-only"
	AgentPresetComment   = "comment-only"
	AgentPresetReadWrite = "read-write"
)

var agentPresets = map[string]APIPermissions{
	AgentPresetReadOnly: {
		"mcp":            {"access"},
		"tasks":          {"read_all", "read_one"},
		"tasks_comments": {"read_all"},
		"projects":       {"read_all", "read_one"},
	},
	AgentPresetComment: {
		"mcp":            {"access"},
		"tasks":          {"read_all", "read_one"},
		"tasks_comments": {"read_all", "create"},
		"projects":       {"read_all", "read_one"},
	},
	AgentPresetReadWrite: {
		"mcp":            {"access"},
		"tasks":          {"read_all", "read_one", "create", "update", "delete"},
		"tasks_comments": {"read_all", "create"},
		"projects":       {"read_all", "read_one", "update"},
		"labels":         {"read_all", "create"},
	},
}

// AgentProjectMembership is one project an agent is scoped to, plus the
// permission level the agent has inside it.
type AgentProjectMembership struct {
	// The id of the project.
	ProjectID int64 `json:"project_id" doc:"The id of the project the agent is a member of."`
	// The permission the agent has on this project. 0 = Read only, 1 = Read & Write, 2 = Admin.
	Permission Permission `json:"permission" doc:"The permission the agent has on this project. 0 = Read only, 1 = Read & Write, 2 = Admin."`
	// The project's title, resolved on read.
	Title string `json:"title" readOnly:"true" doc:"The title of the project."`
}

// Agent is the API surface for an AI agent: a bot user acting through an API
// token, scoped to specific projects via memberships. It has no table of its
// own — everything is derived from users, api_tokens and projects_users rows.
type Agent struct {
	// The agent's id. Identical to the underlying bot user's id.
	ID int64 `json:"id" path:"agent" doc:"The unique, numeric id of this agent."`
	// The agent's username. Always prefixed with bot-.
	Username string `json:"username" readOnly:"true" doc:"The agent's username. Always prefixed with bot-."`
	// A human-readable name for this agent.
	Name string `json:"name" doc:"A human-readable name for this agent."`
	// The agent's status: 0 = active, 2 = disabled.
	Status user.Status `json:"status" readOnly:"true" doc:"The agent's status: 0 = active, 2 = disabled."`
	// The projects this agent can work on.
	Projects []AgentProjectMembership `json:"projects" doc:"The projects this agent is scoped to. The authenticated user must be admin of each project."`
	// The API tokens issued for this agent (secrets never included).
	Tokens []*APIToken `json:"tokens" readOnly:"true" doc:"The API tokens issued for this agent. The cleartext token is never included."`
	// When the agent last authenticated a request with any of its tokens. Zero value means never.
	LastUsedAt time.Time `json:"last_used_at,omitempty" readOnly:"true" doc:"When the agent last used any of its tokens. Zero value means never."`
	// A timestamp when this agent was created.
	Created time.Time `json:"created" readOnly:"true" doc:"A timestamp when this agent was created."`
}

// AgentCreate is the request body for provisioning an agent in one step.
type AgentCreate struct {
	// A human-readable name for the agent.
	Name string `json:"name" minLength:"1" maxLength:"250" doc:"A human-readable name for the agent."`
	// The preset defining what the agent can do. One of read-only, comment-only, read-write.
	Preset string `json:"preset" enum:"read-only,comment-only,read-write" doc:"The preset defining what the agent is allowed to do. One of read-only, comment-only, read-write."`
	// The projects the agent gets access to. Requires admin permission on each.
	Projects []AgentProjectMembership `json:"projects" doc:"The projects the agent is scoped to, each with a permission level."`
	// Optional username; derived from the name if omitted.
	Username string `json:"username,omitempty" doc:"Optional username for the bot user. Must be prefixed with bot-; the prefix is added automatically."`
	// When the issued token expires. Defaults to 90 days from now.
	ExpiresAt time.Time `json:"expires_at,omitempty" doc:"When the agent's token expires. Defaults to 90 days from now."`
}

// ErrInvalidAgentPreset is returned when an unknown preset name is passed.
type ErrInvalidAgentPreset struct {
	Preset string
}

func (err *ErrInvalidAgentPreset) Error() string {
	return fmt.Sprintf("invalid agent preset %q, must be one of read-only, comment-only, read-write", err.Preset)
}

// ErrCodeInvalidAgentPreset holds the unique world-error code of this error.
const ErrCodeInvalidAgentPreset = 1040

// HTTPError holds the http representation of this error.
func (err *ErrInvalidAgentPreset) HTTPError() *web.HTTPError {
	return &web.HTTPError{HTTPCode: 400, Code: ErrCodeInvalidAgentPreset, Message: err.Error()}
}

var agentNameSlugRegex = regexp.MustCompile(`[^a-z0-9]+`)

func agentUsernameFromName(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = agentNameSlugRegex.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "agent"
	}
	return "bot-" + slug
}

// AgentPermissionsForPreset returns the API-token permission map for a preset.
func AgentPermissionsForPreset(preset string) (APIPermissions, error) {
	perms, ok := agentPresets[preset]
	if !ok {
		return nil, &ErrInvalidAgentPreset{Preset: preset}
	}
	// Copy so callers cannot mutate the preset tables.
	out := make(APIPermissions, len(perms))
	for group, list := range perms {
		out[group] = append([]string(nil), list...)
	}
	return out, nil
}

// CreateAgent provisions a full agent atomically: bot user, scoped API token
// with the preset's route permissions, and project memberships. Returns the
// persisted agent and the cleartext token (shown exactly once).
func CreateAgent(s *xorm.Session, a web.Auth, in *AgentCreate) (*Agent, string, error) {
	owner, ok := a.(*user.User)
	if !ok || owner.IsBot() {
		return nil, "", ErrGenericForbidden{}
	}

	perms, err := AgentPermissionsForPreset(in.Preset)
	if err != nil {
		return nil, "", err
	}

	username := in.Username
	if username == "" {
		username = agentUsernameFromName(in.Name)
	}
	if !strings.HasPrefix(username, "bot-") {
		username = "bot-" + username
	}

	// Retry with a random suffix if the derived username is taken.
	bot := &user.User{Username: username, Name: in.Name}
	created, err := user.CreateBotUser(s, bot, owner)
	if err != nil {
		var exists user.ErrUsernameExists
		if !errors.As(err, &exists) {
			return nil, "", err
		}
		suffix, randErr := utils.CryptoRandomString(4)
		if randErr != nil {
			return nil, "", randErr
		}
		bot = &user.User{Username: username + "-" + strings.ToLower(suffix), Name: in.Name}
		created, err = user.CreateBotUser(s, bot, owner)
		if err != nil {
			return nil, "", err
		}
	}

	expiresAt := in.ExpiresAt
	if expiresAt.IsZero() {
		expiresAt = time.Now().AddDate(0, 0, 90)
	}

	token := &APIToken{
		Title:          in.Name,
		OwnerID:        created.ID,
		APIPermissions: perms,
		ExpiresAt:      expiresAt,
	}
	if err := token.Create(s, owner); err != nil {
		return nil, "", err
	}

	for _, pm := range in.Projects {
		p := &Project{ID: pm.ProjectID}
		isAdmin, err := p.IsAdmin(s, a)
		if err != nil {
			return nil, "", err
		}
		if !isAdmin {
			return nil, "", &ErrNeedToHaveProjectReadAccess{ProjectID: pm.ProjectID}
		}
		pu := &ProjectUser{
			UserID:     created.ID,
			ProjectID:  pm.ProjectID,
			Permission: pm.Permission,
		}
		if _, err := s.Insert(pu); err != nil {
			return nil, "", err
		}
	}

	agent := &Agent{ID: created.ID}
	if err := agent.load(s, created, []*APIToken{token}); err != nil {
		return nil, "", err
	}
	return agent, token.Token, nil
}

// load fills an Agent from its bot user. tokens may be pre-fetched (create
// flow); nil means load from the database.
func (ag *Agent) load(s *xorm.Session, bot *user.User, tokens []*APIToken) error {
	ag.Username = bot.Username
	ag.Name = bot.Name
	ag.Status = bot.Status
	ag.Created = bot.Created

	if tokens == nil {
		tokens = []*APIToken{}
		if err := s.Where("owner_id = ?", ag.ID).Find(&tokens); err != nil {
			return err
		}
	}
	ag.Tokens = tokens
	for _, t := range tokens {
		if t.LastUsedAt.After(ag.LastUsedAt) {
			ag.LastUsedAt = t.LastUsedAt
		}
	}

	memberships := []*ProjectUser{}
	if err := s.Where("user_id = ?", ag.ID).Find(&memberships); err != nil {
		return err
	}
	titles := map[int64]string{}
	if len(memberships) > 0 {
		ids := make([]int64, 0, len(memberships))
		for _, m := range memberships {
			ids = append(ids, m.ProjectID)
		}
		projects := []*Project{}
		if err := s.In("id", ids).Find(&projects); err != nil {
			return err
		}
		for _, p := range projects {
			titles[p.ID] = p.Title
		}
	}
	ag.Projects = make([]AgentProjectMembership, 0, len(memberships))
	for _, m := range memberships {
		ag.Projects = append(ag.Projects, AgentProjectMembership{
			ProjectID:  m.ProjectID,
			Permission: m.Permission,
			Title:      titles[m.ProjectID],
		})
	}
	return nil
}

// GetAgentAgent loads one agent for its owner. Ownership is enforced via
// canManageAgent.
func GetAgent(s *xorm.Session, a web.Auth, id int64) (*Agent, error) {
	if err := canManageAgent(s, a, id); err != nil {
		return nil, err
	}
	bot, err := user.GetUserByID(s, id)
	if err != nil {
		return nil, err
	}
	agent := &Agent{ID: id}
	if err := agent.load(s, bot, nil); err != nil {
		return nil, err
	}
	return agent, nil
}

// ListAgents returns all agents owned by the caller.
func ListAgents(s *xorm.Session, a web.Auth) ([]*Agent, error) {
	bots := []*user.User{}
	if err := s.Where("bot_owner_id = ?", a.GetID()).Find(&bots); err != nil {
		return nil, err
	}
	agents := make([]*Agent, 0, len(bots))
	for _, bot := range bots {
		agent := &Agent{ID: bot.ID}
		if err := agent.load(s, bot, nil); err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	return agents, nil
}

// DeleteAgent removes the agent's bot user and with it its tokens and
// project memberships.
func DeleteAgent(s *xorm.Session, a web.Auth, id int64) error {
	if err := canManageAgent(s, a, id); err != nil {
		return err
	}
	bot, err := user.GetUserByID(s, id)
	if err != nil {
		return err
	}
	// DeleteUser has no projects_users cleanup of its own; memberships are the
	// agent's access scope, so they must not outlive it.
	if _, err := s.Where("user_id = ?", id).Delete(&ProjectUser{}); err != nil {
		return err
	}
	return DeleteUser(s, bot)
}

// RotateAgentToken revokes all of the agent's existing tokens and issues a
// fresh one with the given (or previous) preset. Returns the updated agent and
// the cleartext token, visible exactly once.
func RotateAgentToken(s *xorm.Session, a web.Auth, id int64, preset string) (*Agent, string, error) {
	if err := canManageAgent(s, a, id); err != nil {
		return nil, "", err
	}

	var oldTokens []*APIToken
	if err := s.Where("owner_id = ?", id).Find(&oldTokens); err != nil {
		return nil, "", err
	}

	if preset == "" && len(oldTokens) > 0 {
		// Reuse the previous token's permissions verbatim: rotating must never
		// silently widen or narrow an agent's rights.
		preset = "custom"
		for _, t := range oldTokens {
			// Try to recognise the permission map as one of the presets; if it
			// matches none, keep "custom" and copy the map directly below.
			for name, presetPerms := range agentPresets {
				if permissionsEqual(t.APIPermissions, presetPerms) {
					preset = name
					break
				}
			}
			if preset != "custom" {
				break
			}
		}
	}

	var perms APIPermissions
	if preset == "custom" {
		if len(oldTokens) == 0 {
			return nil, "", &ErrInvalidAgentPreset{Preset: preset}
		}
		perms = oldTokens[0].APIPermissions
	} else {
		var err error
		perms, err = AgentPermissionsForPreset(preset)
		if err != nil {
			return nil, "", err
		}
	}

	owner, err := user.GetUserByID(s, a.GetID())
	if err != nil {
		return nil, "", err
	}
	bot, err := user.GetUserByID(s, id)
	if err != nil {
		return nil, "", err
	}

	var expiresAt time.Time
	if len(oldTokens) > 0 {
		expiresAt = oldTokens[0].ExpiresAt
	}
	if expiresAt.IsZero() || expiresAt.Before(time.Now()) {
		expiresAt = time.Now().AddDate(0, 0, 90)
	}

	for _, t := range oldTokens {
		if _, err := s.ID(t.ID).Delete(&APIToken{}); err != nil {
			return nil, "", err
		}
	}

	token := &APIToken{
		Title:          bot.Name,
		OwnerID:        id,
		APIPermissions: perms,
		ExpiresAt:      expiresAt,
	}
	if err := token.Create(s, owner); err != nil {
		return nil, "", err
	}

	agent := &Agent{ID: id}
	if err := agent.load(s, bot, []*APIToken{token}); err != nil {
		return nil, "", err
	}
	return agent, token.Token, nil
}

func permissionsEqual(a, b APIPermissions) bool {
	if len(a) != len(b) {
		return false
	}
	for group, perms := range a {
		other, ok := b[group]
		if !ok || len(perms) != len(other) {
			return false
		}
		counts := map[string]int{}
		for _, p := range perms {
			counts[p]++
		}
		for _, p := range other {
			counts[p]--
		}
		for _, c := range counts {
			if c != 0 {
				return false
			}
		}
	}
	return true
}

// canManageAgent verifies the caller owns the bot user behind the agent.
func canManageAgent(s *xorm.Session, a web.Auth, id int64) error {
	if _, is := a.(*LinkSharing); is {
		return ErrGenericForbidden{}
	}
	bot, err := user.GetUserByID(s, id)
	if err != nil {
		return err
	}
	if !bot.IsBot() || bot.BotOwnerID != a.GetID() {
		return ErrGenericForbidden{}
	}
	return nil
}
