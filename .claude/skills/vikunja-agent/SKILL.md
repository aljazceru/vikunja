---
name: vikunja-agent
description: Use when a human asks to set up, manage, or inspect AI agent access in Vikunja — provisioning agents, scoping them to projects, rotating or revoking their tokens, or checking agent heartbeats. Covers the POST /api/v2/agents endpoints, presets, and the Agents settings page.
user-invocable: true
---

# Managing AI agents (operator side)

An **agent** is a bundle of three things created in one step: a bot user, an API
token restricted by a preset, and memberships in the projects you pick. Agents
authenticate with their token and are visible as regular members — humans see
what they do on the boards.

## Provision an agent

```bash
curl -X POST "$API/api/v2/agents" \
  -H "Authorization: Bearer $USER_JWT" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "research-agent",
    "preset": "read-write",
    "projects": [{"project_id": 42, "permission": 1}]
  }'
```

Response: `{agent: {...}, token: "tk_..."}` — **the token is shown exactly
once**, at creation. Store it immediately (env var, secret store). There is no
way to retrieve it later; rotation is the recovery path.

- `preset` — what the agent may do:
  - `read-only` — see tasks/projects, touch nothing
  - `comment-only` — read everything + report via comments
  - `read-write` — claim/update/complete tasks, comment, create tasks/labels
- `projects[].permission` — `0` read, `1` read/write, `2` admin, **within each
  listed project**. The caller must be admin of every listed project.
- `expires_at` (optional, RFC3339) — token expiry, defaults to 90 days.

The agent's username derives from the name (`Research Agent` → `bot-research-agent`).

## Manage agents

| Action | Call |
|---|---|
| List (with memberships, tokens, last-used heartbeat) | `GET /api/v2/agents` |
| Read one | `GET /api/v2/agents/{id}` |
| Rotate token (revokes all old ones; keeps preset unless a new one is passed) | `POST /api/v2/agents/{id}/rotate-token` |
| Delete agent (revokes token, removes memberships) | `DELETE /api/v2/agents/{id}` |

Only the agent's owner may do any of these. Agents are managed in the UI under
**Settings → Agents** (same operations, token-copy screen on create/rotate).

## Heartbeats

Every agent carries `last_used_at` (updated at most once a minute when its
token authenticates). A stale heartbeat on an agent that should be working
means the agent runner is down or the token expired — rotate to recover.

## Changing an agent's scope

Project scope is plain membership: use the normal project-sharing endpoints
(`PUT /projects/{id}/users`) with the bot's username to add the agent to more
projects, or DELETE to remove it. Changes take effect immediately — no token
rotation needed. To also change *what* it may do, rotate with a different
preset.
