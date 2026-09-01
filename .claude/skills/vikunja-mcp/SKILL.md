---
name: vikunja-mcp
description: Use when an agent (you) needs to work on Vikunja tasks through the MCP endpoint at /api/v2/mcp — finding work, claiming tasks, posting progress, completing tasks. Covers tool availability per token preset, the claim workflow that moves cards on the kanban board, and scope rules.
user-invocable: true
---

# Working on Vikunja via MCP

You are one agent among possibly many humans and agents. Your token decides
what you can do; your project memberships decide where. Everything you do is
visible to the project's members in real time — claim visibly, comment often.

## Connecting

- Endpoint: `https://<instance>/api/v2/mcp` (streamable HTTP)
- Auth: `Authorization: Bearer tk_...` (an agent token with the `mcp.access`
  permission — every agent preset includes it)
- Tools you cannot use (missing token permission) simply don't appear in
  `tools/list`.

## The claim workflow

1. `whoami` — orient: your id, your permissions, your projects (with your
   permission level in each).
2. `list_tasks` — find work. Useful filters: `project_id`, `done: false`,
   `unassigned: true` (the pool), `assignee_id: -1` (your own tasks),
   `updated_since` for incremental sync.
3. `get_task` — read the full task (description, comments, buckets) before
   touching it.
4. **`assign_to_me`** — the moment you start work. It assigns the task to you
   and moves it to the project's "In Progress" bucket, so everyone watching
   the board sees that this task is being worked on. If the project has no
   such bucket, assignment still happens; the move needs
   `set_task_in_progress` with an explicit `bucket_id`.
5. `add_comment` — report progress and findings as you go. This is how humans
   and other agents follow your work; don't work silently.
6. `complete_task` when done (post a final summary comment first). Completion
   moves the card into the done bucket automatically.
7. `unassign_me` if you cannot finish — always with a comment explaining the
   state so the next taker doesn't repeat your dead ends.

## Scope rules (non-negotiable)

- You only ever see and touch tasks in projects your agent is a member of.
  Out-of-scope ids return a tool error — that's expected, don't retry with
  different ids.
- The `read-only` preset cannot claim or comment; `comment-only` cannot claim.
  If your workflow requires more, ask your operator to rotate the token with a
  wider preset (they'll know what this means).
- Never invent project/task ids — look them up (`whoami`, `list_projects`,
  `list_tasks`) first.
- Don't create tasks unless explicitly asked (`create_task` is deliberately
  narrow); humans structure the work, agents execute it.

## Tool reference

| Tool | Needs preset | Notes |
|---|---|---|
| `whoami` | any | identity, scopes, projects |
| `list_projects` | any | id + title of accessible projects |
| `get_project` | any | incl. views (kanban view ids for buckets) |
| `list_tasks` | any | filters; newest first |
| `get_task` | any | full detail incl. comments + buckets |
| `create_task` | read-write | only when asked |
| `update_task` | read-write | field patch (title, description, due, priority, percent) |
| `assign_to_me` | read-write | claim + move to In Progress |
| `unassign_me` | read-write | release back to pool |
| `set_task_in_progress` | read-write | explicit bucket move |
| `complete_task` | read-write | done + done-bucket move |
| `add_comment` | comment-only+ | progress reporting |
