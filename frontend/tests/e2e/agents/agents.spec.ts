import {test, expect} from '../../support/fixtures'
import {ProjectFactory} from '../../factories/project'
import {ProjectViewFactory} from '../../factories/project_view'
import {BucketFactory} from '../../factories/bucket'
import {TaskFactory} from '../../factories/task'
import {TaskBucketFactory} from '../../factories/task_buckets'

const API_ROOT = (process.env.API_URL || 'http://localhost:3456/api/v1').replace(/\/api\/v1\/?$/, '')

// A kanban project with a To Do / In Progress / Done board and one task in
// To Do — the board an agent will work on. Tables are empty at test start, so
// autoincrement ids are deterministic: this view is id 1, its buckets are
// ids 1–3 — which lets the view row carry done_bucket_id at creation (the
// testing endpoint can only insert rows, not update them).
async function seedAgentProject() {
	const [project] = await ProjectFactory.create(1)
	const buckets = await BucketFactory.create(3, {
		project_view_id: 1,
		title: i => ['To Do', 'In Progress', 'Done'][i - 1],
	})
	const [view] = await ProjectViewFactory.create(1, {
		project_id: project.id,
		view_kind: 3,
		bucket_configuration_mode: 1,
		done_bucket_id: buckets[2].id,
	})
	const [task] = await TaskFactory.create(1, {
		project_id: project.id,
		index: 1,
	})
	await TaskBucketFactory.create(1, {
		task_id: task.id,
		bucket_id: buckets[0].id,
		project_view_id: view.id,
	}, false)
	return {project, view, buckets, task}
}

async function provisionAgent(apiContext, token: string, projectId: number) {
	const response = await apiContext.post('../v2/agents', {
		data: {
			name: 'E2E Agent',
			preset: 'read-write',
			projects: [{project_id: projectId, permission: 1}],
		},
		headers: {Authorization: `Bearer ${token}`},
	})
	expect(response.status()).toBe(201)
	const body = await response.json()
	return {agentId: body.agent.id, token: body.token}
}

// Minimal MCP JSON-RPC call over streamable http.
async function mcpCall(apiContext, token: string, id: number, method: string, params: object = {}) {
	const response = await apiContext.fetch(`${API_ROOT}/api/v2/mcp`, {
		method: 'POST',
		data: {jsonrpc: '2.0', id, method, params},
		headers: {
			Authorization: `Bearer ${token}`,
			Accept: 'application/json, text/event-stream',
		},
	})
	expect(response.status()).toBe(200)
	return response.json()
}

test.describe('Agents', () => {

	test('creating an agent through the UI shows the token once', async ({authenticatedPage: page}) => {
		await ProjectFactory.create(1, {title: 'Agent Playground'})

		await page.goto('/user/settings/agents')
		await page.waitForLoadState('networkidle')

		await page.locator('button').filter({hasText: 'Create agent'}).click()
		await page.locator('.agent-form input.input').first().fill('UI Test Agent')
		// read-write is the default preset; pick the project scope.
		await page.locator('.agent-form .multiselect input').first().click()
		await page.locator('.agent-form .multiselect .search-result-button').first().click()
		await page.locator('.agent-form button').filter({hasText: 'Create agent'}).click()

		await expect(page.locator('.agent-created code.agent-token')).toBeVisible({timeout: 10000})
		await expect(page.locator('.agent-created code.agent-token')).toContainText('tk_')
	})

	test('agent claim moves the task to In Progress live on the kanban board', async ({
		authenticatedPage: page,
		apiContext,
		userToken,
	}) => {
		const {project, view, buckets, task} = await seedAgentProject()
		const {token} = await provisionAgent(apiContext, userToken, project.id)

		// The human watches the board.
		await page.goto(`/projects/${project.id}/${view.id}`)
		await expect(page.locator('.kanban .bucket .title').filter({hasText: 'To Do'})).toBeVisible()
		await expect(page.locator('.kanban .bucket').first()).toContainText(task.title)
		await page.waitForLoadState('networkidle')

		// The agent orients itself and claims the task over MCP.
		const init = await mcpCall(apiContext, token, 1, 'initialize', {
			protocolVersion: '2025-06-18',
			capabilities: {},
			clientInfo: {name: 'e2e-agent', version: '1.0'},
		})
		expect(init.result.serverInfo.name).toBe('vikunja')

		const claim = await mcpCall(apiContext, token, 2, 'tools/call', {
			name: 'assign_to_me',
			arguments: {task_id: task.id},
		})
		expect(claim.result.isError).toBeUndefined()

		// The card jumps to the In Progress column without any reload.
		const inProgressColumn = page.locator('.kanban .bucket').filter({hasText: 'In Progress'})
		await expect(inProgressColumn).toContainText(task.title, {timeout: 10000})

		// And the move actually happened in the backend, not just visually.
		const response = await apiContext.get(`tasks/${task.id}`, {
			headers: {Authorization: `Bearer ${userToken}`},
			params: {expand: 'buckets'},
		})
		const updated = await response.json()
		expect(updated.buckets.map(b => b.id)).toContain(buckets[1].id)
	})

	test('agent finishes work: comment and completion land without reload', async ({
		authenticatedPage: page,
		apiContext,
		userToken,
	}) => {
		const {project, view, buckets, task} = await seedAgentProject()
		const {token} = await provisionAgent(apiContext, userToken, project.id)

		await page.goto(`/projects/${project.id}/${view.id}`)
		await expect(page.locator('.kanban .bucket').first()).toContainText(task.title)
		await page.waitForLoadState('networkidle')

		await mcpCall(apiContext, token, 1, 'initialize', {
			protocolVersion: '2025-06-18',
			capabilities: {},
			clientInfo: {name: 'e2e-agent', version: '1.0'},
		})
		await mcpCall(apiContext, token, 2, 'tools/call', {
			name: 'assign_to_me',
			arguments: {task_id: task.id},
		})
		const inProgressColumn = page.locator('.kanban .bucket').filter({hasText: 'In Progress'})
		await expect(inProgressColumn).toContainText(task.title, {timeout: 10000})

		await mcpCall(apiContext, token, 3, 'tools/call', {
			name: 'add_comment',
			arguments: {task_id: task.id, comment: 'Investigated, ready to finish.'},
		})
		await mcpCall(apiContext, token, 4, 'tools/call', {
			name: 'complete_task',
			arguments: {task_id: task.id},
		})

		// Completing moves the card into the done column live.
		const doneColumn = page.locator('.kanban .bucket').filter({hasText: 'Done'})
		await expect(doneColumn).toContainText(task.title, {timeout: 10000})

		const response = await apiContext.get(`tasks/${task.id}`, {
			headers: {Authorization: `Bearer ${userToken}`},
			params: {expand: 'buckets'},
		})
		const updated = await response.json()
		expect(updated.done).toBe(true)
		expect(updated.buckets.map(b => b.id)).toContain(buckets[2].id)
	})

	test('agent cannot touch a project it has no access to', async ({
		authenticatedPage: page,
		apiContext,
		userToken,
	}) => {
		const {project, view, task} = await seedAgentProject()
		// Explicit ids throughout: '{increment}' restarts at 1 on every factory
		// call (each create truncates), which collides with the seeded rows above.
		const [foreignProject] = await ProjectFactory.create(1, {id: 2}, false)
		const [foreignView] = await ProjectViewFactory.create(1, {
			id: 2,
			project_id: foreignProject.id,
			view_kind: 3,
			bucket_configuration_mode: 1,
		}, false)
		const [foreignBucket] = await BucketFactory.create(1, {
			id: 4,
			project_view_id: foreignView.id,
		}, false)
		const [foreignTask] = await TaskFactory.create(1, {
			id: 2,
			project_id: foreignProject.id,
		}, false)
		await TaskBucketFactory.create(1, {
			task_id: foreignTask.id,
			bucket_id: foreignBucket.id,
			project_view_id: foreignView.id,
		}, false)

		const {token} = await provisionAgent(apiContext, userToken, project.id)
		await mcpCall(apiContext, token, 1, 'initialize', {
			protocolVersion: '2025-06-18',
			capabilities: {},
			clientInfo: {name: 'e2e-agent', version: '1.0'},
		})

		// The agent can see its own project's task…
		const own = await mcpCall(apiContext, token, 2, 'tools/call', {
			name: 'get_task',
			arguments: {id: task.id},
		})
		expect(own.result.isError).toBeUndefined()

		// …but the foreign project is invisible and untouchable.
		const foreign = await mcpCall(apiContext, token, 3, 'tools/call', {
			name: 'get_task',
			arguments: {id: foreignTask.id},
		})
		expect(foreign.result.isError).toBe(true)

		const claim = await mcpCall(apiContext, token, 4, 'tools/call', {
			name: 'assign_to_me',
			arguments: {task_id: foreignTask.id},
		})
		expect(claim.result.isError).toBe(true)

		// Board of the agent's project still renders fine.
		await page.goto(`/projects/${project.id}/${view.id}`)
		await expect(page.locator('.kanban .bucket').first()).toContainText(task.title)
	})
})
