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

// Package mcp exposes Vikunja as a Model Context Protocol server at
// /api/v2/mcp. Authentication is an API token with the mcp.access permission;
// tool visibility is filtered by the token's route permissions, and every tool
// additionally goes through the model-layer permission checks (project
// memberships), so an MCP client can never exceed what its token + memberships
// allow.
package mcp

import (
	"context"
	"net/http"
	"strings"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/log"
	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/user"
)

// Auth carries the authenticated API token and its owner through a request.
type Auth struct {
	Token *models.APIToken
	User  *user.User
}

type authContextKey struct{}

// AuthFromContext extracts the MCP auth from a request context.
func AuthFromContext(ctx context.Context) *Auth {
	a, _ := ctx.Value(authContextKey{}).(*Auth)
	return a
}

// authenticate validates the Bearer API token of an MCP request. It returns
// (nil, nil) when the token is merely invalid — the caller answers with 401.
func authenticate(r *http.Request) (*Auth, error) {
	header := r.Header.Get("Authorization")
	raw := strings.TrimPrefix(header, "Bearer ")
	if header == "" || raw == header || !strings.HasPrefix(raw, models.APITokenPrefix) {
		return nil, nil
	}

	s := db.NewSession()
	defer s.Close()

	token, u, err := models.ValidateTokenAndGetOwner(s, raw)
	if err != nil {
		return nil, err
	}
	if token == nil || u == nil {
		return nil, nil
	}
	if !token.HasMCPAccess() {
		log.Debugf("[mcp auth] API token %d does not have mcp access permission", token.ID)
		return nil, nil
	}
	return &Auth{Token: token, User: u}, nil
}

// tokenHasPermission reports whether the token's route permissions include
// group/perm (e.g. tasks/update). MCP tools are only registered for tokens
// that hold their required permission.
func tokenHasPermission(token *models.APIToken, group, perm string) bool {
	perms, has := token.APIPermissions[group]
	if !has {
		return false
	}
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}
