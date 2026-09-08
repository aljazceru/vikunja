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

package apiv2

import (
	"context"
	"net/http"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/models"

	"github.com/danielgtaylor/huma/v2"
)

type agentListBody struct {
	Body Paginated[*models.Agent]
}

// agentWithToken is the one-time response of provisioning or rotating: the
// agent plus the cleartext token, which is never shown again.
type agentWithToken struct {
	Agent *models.Agent `json:"agent" doc:"The provisioned agent."`
	Token string        `json:"token" doc:"The cleartext API token. Returned only once, in this response; never readable again."`
}

// RegisterAgentRoutes wires the one-step agent provisioning endpoints onto the
// Huma API. Agents are orchestrations over bot users, API tokens and project
// memberships rather than a CRUDable table, so these are custom handlers.
func RegisterAgentRoutes(api huma.API) {
	tags := []string{"agents"}

	Register(api, huma.Operation{
		OperationID: "agents-create",
		Summary:     "Provision an agent",
		Description: "Provisions an AI agent in one atomic request: a bot user, an API token restricted to the preset's route permissions, and memberships in the given projects. The caller must be admin of every listed project and becomes the agent's owner. Returns the agent and the cleartext token — the token is never shown again. Bots and link shares cannot create agents.",
		Method:      http.MethodPost,
		Path:        "/agents",
		Tags:        tags,
	}, agentsCreate)

	Register(api, huma.Operation{
		OperationID: "agents-list",
		Summary:     "List agents",
		Description: "Returns all agents owned by the authenticated user, with their project memberships, tokens (no secrets) and last activity.",
		Method:      http.MethodGet,
		Path:        "/agents",
		Tags:        tags,
	}, agentsList)

	Register(api, huma.Operation{
		OperationID: "agents-read",
		Summary:     "Get an agent",
		Description: "Returns one agent owned by the authenticated user. Agents owned by anyone else are refused with 403.",
		Method:      http.MethodGet,
		Path:        "/agents/{agent}",
		Tags:        tags,
	}, agentsRead)

	Register(api, huma.Operation{
		OperationID: "agents-rotate-token",
		Summary:     "Rotate an agent's token",
		Description: "Revokes all of the agent's existing tokens and issues a fresh one. Without a preset in the body the new token keeps the previous token's permissions; otherwise the given preset applies. Returns the cleartext token once.",
		Method:      http.MethodPost,
		Path:        "/agents/{agent}/rotate-token",
		Tags:        tags,
	}, agentsRotateToken)

	Register(api, huma.Operation{
		OperationID: "agents-delete",
		Summary:     "Delete an agent",
		Description: "Permanently deletes the agent's bot user together with its tokens and project memberships. Only the owner may delete it.",
		Method:      http.MethodDelete,
		Path:        "/agents/{agent}",
		Tags:        tags,
	}, agentsDelete)
}

func init() { AddRouteRegistrar(RegisterAgentRoutes) }

func agentsCreate(ctx context.Context, in *struct {
	Body models.AgentCreate
}) (*struct{ Body agentWithToken }, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	s := db.NewSession()
	defer s.Close()

	agent, token, err := models.CreateAgent(s, a, &in.Body)
	if err != nil {
		_ = s.Rollback()
		return nil, translateDomainError(err)
	}
	if err := s.Commit(); err != nil {
		return nil, translateDomainError(err)
	}
	return &struct{ Body agentWithToken }{Body: agentWithToken{Agent: agent, Token: token}}, nil
}

func agentsList(ctx context.Context, in *ListParams) (*agentListBody, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	s := db.NewSession()
	defer s.Close()

	agents, err := models.ListAgents(s, a)
	if err != nil {
		return nil, translateDomainError(err)
	}
	return &agentListBody{Body: NewPaginated(agents, int64(len(agents)), in.Page, in.PerPage)}, nil
}

func agentsRead(ctx context.Context, in *struct {
	Agent int64 `path:"agent" doc:"The numeric id of the agent."`
}) (*singleBody[models.Agent], error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	s := db.NewSession()
	defer s.Close()

	agent, err := models.GetAgent(s, a, in.Agent)
	if err != nil {
		return nil, translateDomainError(err)
	}
	return &singleBody[models.Agent]{Body: agent}, nil
}

func agentsRotateToken(ctx context.Context, in *struct {
	Agent int64 `path:"agent" doc:"The numeric id of the agent."`
	Body  struct {
		// The preset for the new token. Defaults to the previous token's permissions.
		Preset string `json:"preset,omitempty" enum:"read-only,comment-only,read-write" doc:"The preset for the new token. If omitted, the new token keeps the previous token's permissions."`
	}
}) (*struct{ Body agentWithToken }, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	s := db.NewSession()
	defer s.Close()

	agent, token, err := models.RotateAgentToken(s, a, in.Agent, in.Body.Preset)
	if err != nil {
		_ = s.Rollback()
		return nil, translateDomainError(err)
	}
	if err := s.Commit(); err != nil {
		return nil, translateDomainError(err)
	}
	return &struct{ Body agentWithToken }{Body: agentWithToken{Agent: agent, Token: token}}, nil
}

func agentsDelete(ctx context.Context, in *struct {
	Agent int64 `path:"agent" doc:"The numeric id of the agent."`
}) (*emptyBody, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	s := db.NewSession()
	defer s.Close()

	if err := models.DeleteAgent(s, a, in.Agent); err != nil {
		_ = s.Rollback()
		return nil, translateDomainError(err)
	}
	if err := s.Commit(); err != nil {
		return nil, translateDomainError(err)
	}
	return &emptyBody{}, nil
}
