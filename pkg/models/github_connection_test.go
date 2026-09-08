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

	"code.vikunja.io/api/pkg/web"
)

func TestGitHubConnection_CanDoSomething(t *testing.T) {
	tests := []struct {
		name    string
		conn    *GitHubConnection
		auth    web.Auth
		want    map[string]bool
		wantErr bool
	}{
		{
			name: "project owner",
			conn: &GitHubConnection{ProjectID: 1},
			auth: &user.User{ID: 1},
			want: map[string]bool{"CanCreate": true, "CanUpdate": true, "CanDelete": true},
		},
		{
			name: "read-only share",
			conn: &GitHubConnection{ProjectID: 9},
			auth: &user.User{ID: 1},
			want: map[string]bool{"CanCreate": false, "CanUpdate": false, "CanDelete": false},
		},
		{
			name: "write share",
			conn: &GitHubConnection{ProjectID: 10},
			auth: &user.User{ID: 1},
			want: map[string]bool{"CanCreate": true, "CanUpdate": true, "CanDelete": true},
		},
		{
			name: "admin share",
			conn: &GitHubConnection{ProjectID: 11},
			auth: &user.User{ID: 1},
			want: map[string]bool{"CanCreate": true, "CanUpdate": true, "CanDelete": true},
		},
		{
			name: "no access to project",
			conn: &GitHubConnection{ProjectID: 2},
			auth: &user.User{ID: 1},
			want: map[string]bool{"CanCreate": false, "CanUpdate": false, "CanDelete": false},
		},
		{
			name: "nonexistent project",
			conn: &GitHubConnection{ProjectID: 424242},
			auth: &user.User{ID: 1},
			want: map[string]bool{"CanCreate": false, "CanUpdate": false, "CanDelete": false},
		},
		{
			name: "by id resolves the project (write share)",
			conn: &GitHubConnection{ID: 3},
			auth: &user.User{ID: 1},
			want: map[string]bool{"CanCreate": true, "CanUpdate": true, "CanDelete": true},
		},
		{
			name: "by id resolves the project (no share)",
			conn: &GitHubConnection{ID: 4},
			auth: &user.User{ID: 1},
			want: map[string]bool{"CanCreate": false, "CanUpdate": false, "CanDelete": false},
		},
		{
			name: "by nonexistent id",
			conn: &GitHubConnection{ID: 424242},
			auth: &user.User{ID: 1},
			want: map[string]bool{"CanCreate": false, "CanUpdate": false, "CanDelete": false},
		},
		{
			name: "link share auth is always rejected",
			conn: &GitHubConnection{ProjectID: 1},
			auth: &LinkSharing{ID: 1},
			want: map[string]bool{"CanCreate": false, "CanUpdate": false, "CanDelete": false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db.LoadAndAssertFixtures(t)
			s := db.NewSession()
			defer s.Close()

			if got, _ := tt.conn.CanCreate(s, tt.auth); got != tt.want["CanCreate"] {
				t.Errorf("CanCreate() = %v, want %v", got, tt.want["CanCreate"])
			}
			if got, _ := tt.conn.CanUpdate(s, tt.auth); got != tt.want["CanUpdate"] {
				t.Errorf("CanUpdate() = %v, want %v", got, tt.want["CanUpdate"])
			}
			if got, _ := tt.conn.CanDelete(s, tt.auth); got != tt.want["CanDelete"] {
				t.Errorf("CanDelete() = %v, want %v", got, tt.want["CanDelete"])
			}
		})
	}
}

func TestGitHubConnection_CanRead(t *testing.T) {
	tests := []struct {
		name    string
		conn    *GitHubConnection
		auth    web.Auth
		want    bool
		wantMax int
	}{
		{
			name: "project owner",
			conn: &GitHubConnection{ID: 1},
			auth: &user.User{ID: 1},
			want: true, wantMax: int(PermissionAdmin),
		},
		{
			name: "read share",
			conn: &GitHubConnection{ID: 2},
			auth: &user.User{ID: 1},
			want: true, wantMax: int(PermissionRead),
		},
		{
			name: "no access",
			conn: &GitHubConnection{ID: 4},
			auth: &user.User{ID: 1},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db.LoadAndAssertFixtures(t)
			s := db.NewSession()
			defer s.Close()

			got, maxPerm, err := tt.conn.CanRead(s, tt.auth)
			if err != nil {
				t.Errorf("CanRead() returned error: %s", err)
				return
			}
			if got != tt.want {
				t.Errorf("CanRead() = %v, want %v", got, tt.want)
			}
			if got && maxPerm != tt.wantMax {
				t.Errorf("CanRead() max permission = %d, want %d", maxPerm, tt.wantMax)
			}
		})
	}
}

func TestGitHubConnection_MasksCredentials(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	defer s.Close()

	conn, err := GetGitHubConnectionByID(s, 1)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if conn.Token != "" {
		t.Errorf("Token not masked, got %q", conn.Token)
	}
	if conn.WebhookSecret != "" {
		t.Errorf("WebhookSecret not masked, got %q", conn.WebhookSecret)
	}
}

func TestGitHubConnection_ReadAll(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	defer s.Close()

	// ReadAll enforces the project's read permission itself (the generic list
	// pipeline doesn't run CanRead).
	for projectID, wantCount := range map[int64]int{1: 1, 9: 1, 10: 1} {
		conn := &GitHubConnection{ProjectID: projectID}
		result, count, _, err := conn.ReadAll(s, &user.User{ID: 1}, "", 1, 20)
		if err != nil {
			t.Fatalf("project %d: unexpected error: %s", projectID, err)
		}
		if count != wantCount {
			t.Errorf("project %d: got %d connections, want %d", projectID, count, wantCount)
		}
		conns, ok := result.([]*GitHubConnection)
		if !ok {
			t.Fatalf("project %d: unexpected result type %T", projectID, result)
		}
		for _, c := range conns {
			if c.Token != "" || c.WebhookSecret != "" {
				t.Errorf("project %d: credentials leaked in ReadAll", projectID)
			}
		}
	}

	// No access to the project → forbidden, not an empty list.
	if _, _, _, err := (&GitHubConnection{ProjectID: 2}).ReadAll(s, &user.User{ID: 1}, "", 1, 20); !IsErrGenericForbidden(err) {
		t.Errorf("ReadAll without project access: expected forbidden, got %v", err)
	}
}
