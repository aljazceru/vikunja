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

package github

import "time"

// Minimal REST payload types for the endpoints this integration touches.
// Only the fields actually consumed are declared.

// Repository is a GitHub repository as returned by the REST API.
type Repository struct {
	ID       int64  `json:"id"`
	NodeID   string `json:"node_id"`
	FullName string `json:"full_name"`
	Owner    *User  `json:"owner"`
	Name     string `json:"name"`
	HTMLURL  string `json:"html_url"`
}

// User is a GitHub account.
type User struct {
	Login   string `json:"login"`
	HTMLURL string `json:"html_url"`
}

// Label is a GitHub label. Color is a hex string without the leading '#'.
type Label struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Milestone is a GitHub milestone.
type Milestone struct {
	DueOn *time.Time `json:"due_on"`
}

// PullRequestRef is the pull-request stub embedded in the issues API
// representation of a pull request. Its presence marks an "issue" as a PR.
type PullRequestRef struct {
	HTMLURL  string     `json:"html_url"`
	MergedAt *time.Time `json:"merged_at"`
}

// Issue is an issue or pull request as returned by the issues endpoints
// (the issues API returns PRs too, distinguished by the PullRequest field).
type Issue struct {
	ID          int64           `json:"id"`
	NodeID      string          `json:"node_id"`
	Number      int64           `json:"number"`
	Title       string          `json:"title"`
	Body        string          `json:"body"`
	State       string          `json:"state"`
	HTMLURL     string          `json:"html_url"`
	Labels      []*Label        `json:"labels"`
	Milestone   *Milestone      `json:"milestone"`
	User        *User           `json:"user"`
	PullRequest *PullRequestRef `json:"pull_request"`
	ClosedAt    *time.Time      `json:"closed_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// IsPullRequest reports whether the issue representation is a pull request.
func (i *Issue) IsPullRequest() bool {
	return i.PullRequest != nil
}

// IsClosed reports whether the item is closed. Merged PRs are closed too.
func (i *Issue) IsClosed() bool {
	return i.State == "closed"
}

// webhookPayload is the common envelope of GitHub webhook deliveries. Only
// fields needed to route the delivery are parsed; the item itself is re-fetched
// through the API so webhooks and reconciliations share one code path.
type webhookPayload struct {
	Action       string           `json:"action"`
	Number       int64            `json:"number"`
	Repository   *Repository      `json:"repository"`
	Installation *installationRef `json:"installation"`
}

type installationRef struct {
	ID int64 `json:"id"`
}

// accessTokenResponse is the answer of POST /app/installations/{id}/access_tokens.
type accessTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// repoHook is a repository webhook as created for PAT connections.
type repoHook struct {
	ID int64 `json:"id"`
}
