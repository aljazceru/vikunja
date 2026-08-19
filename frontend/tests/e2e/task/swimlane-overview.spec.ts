import {expect, test} from '../../support/fixtures'
import {ProjectFactory} from '../../factories/project'
import {TaskFactory} from '../../factories/task'

// Seeds two projects with a mix of tasks that exercise every derived state of
// the swimlane board: plain todo, overdue, in-progress (via percent_done) and
// a task in a second project to verify lane grouping.
async function seedBoard() {
	const projects = await ProjectFactory.create(2)
	const [projectA, projectB] = projects

	const now = new Date()
	const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1000)
	const nextWeek = new Date(now.getTime() + 7 * 24 * 60 * 60 * 1000)
	const iso = (d: Date) => d.toISOString()

	const tasks = [
		{id: 1, index: 1, project_id: projectA.id, created_by_id: 1, title: 'Swimlane plain todo', done: false, percent_done: 0, created: iso(now), updated: iso(now)},
		{id: 2, index: 2, project_id: projectA.id, created_by_id: 1, title: 'Swimlane overdue task', done: false, percent_done: 0, due_date: iso(yesterday), created: iso(now), updated: iso(now)},
		{id: 3, index: 3, project_id: projectA.id, created_by_id: 1, title: 'Swimlane halfway task', done: false, percent_done: 0.5, created: iso(now), updated: iso(now)},
		{id: 4, index: 1, project_id: projectB.id, created_by_id: 1, title: 'Swimlane other project task', done: false, percent_done: 0, due_date: iso(nextWeek), created: iso(now), updated: iso(now)},
	]
	await TaskFactory.seed(TaskFactory.table, tasks)

	return {projectA, projectB, tasks}
}

test.describe('Swimlane overview', () => {

	test('shows one lane per project with the derived columns', async ({authenticatedPage: page}) => {
		const {projectA, projectB} = await seedBoard()

		await page.goto('/tasks/overview')

		const board = page.getByTestId('swimlane-overview')
		await expect(board).toBeVisible()

		const lanes = page.getByTestId('project-lane')
		await expect(lanes).toHaveCount(2)

		const laneTitles = page.getByTestId('lane-title')
		await expect(laneTitles.nth(0)).toContainText(projectA.title)
		await expect(laneTitles.nth(1)).toContainText(projectB.title)

		// Column counts in the header: 3 todo, 1 in progress
		await expect(page.getByTestId('column-title-todo')).toContainText('3')
		await expect(page.getByTestId('column-title-progress')).toContainText('1')

		// Cards land in the right lane + column
		const laneA = page.locator(`[data-cy="project-lane"][data-project-id="${projectA.id}"]`)
		await expect(laneA.getByTestId('swimlane-column-todo').getByTestId('swimlane-card')).toHaveCount(2)
		await expect(laneA.getByTestId('swimlane-column-progress').getByTestId('swimlane-card')).toHaveCount(1)
		const laneB = page.locator(`[data-cy="project-lane"][data-project-id="${projectB.id}"]`)
		await expect(laneB.getByTestId('swimlane-column-todo').getByTestId('swimlane-card')).toHaveCount(1)
	})

	test('marks overdue tasks on the lane and the card', async ({authenticatedPage: page}) => {
		const {projectA} = await seedBoard()

		await page.goto('/tasks/overview')

		const laneA = page.locator(`[data-cy="project-lane"][data-project-id="${projectA.id}"]`)
		await expect(laneA.getByTestId('lane-overdue')).toContainText('1')

		const overdueCard = page.locator('[data-task-id="2"]')
		await expect(overdueCard.locator('.swimlane-card__due.is-overdue')).toBeVisible()
	})

	test('opens the detail pane, moves a task between columns via percent done and closes', async ({authenticatedPage: page}) => {
		const {projectA} = await seedBoard()

		await page.goto('/tasks/overview')

		// Open the in-progress task
		await page.locator('[data-task-id="3"]').click()
		await expect(page.getByTestId('task-detail-pane-title')).toHaveText('Swimlane halfway task')

		// Set priority to URGENT (4) and verify it persists via the API-driven update
		await page.locator('[data-cy="task-detail-pane"] select').nth(1).selectOption('4')
		const laneA = page.locator(`[data-cy="project-lane"][data-project-id="${projectA.id}"]`)
		await expect(page.locator('[data-task-id="3"] .swimlane-card__priority.is-high')).toBeVisible()

		// Drop the progress back to 0 — the card must move to the todo column
		await page.locator('[data-cy="task-detail-pane"] select').first().selectOption('0')
		await expect(laneA.getByTestId('swimlane-column-todo').locator('[data-task-id="3"]')).toBeVisible()
		await expect(laneA.getByTestId('swimlane-column-progress').getByTestId('swimlane-card')).toHaveCount(0)

		// Close the pane
		await page.getByTestId('task-detail-pane-close').click()
		await expect(page.getByTestId('task-detail-pane-title')).not.toBeVisible()
	})

	test('editing the due date from the detail pane updates the card', async ({authenticatedPage: page}) => {
		await seedBoard()

		await page.goto('/tasks/overview')

		// Open the overdue task — it shows a red due chip
		await page.locator('[data-task-id="2"]').click()
		await expect(page.getByTestId('task-detail-pane-title')).toHaveText('Swimlane overdue task')
		await expect(page.locator('[data-task-id="2"] .swimlane-card__due.is-overdue')).toBeVisible()

		// Remove the due date — the overdue chip on the card must disappear
		await page.locator('.task-detail-pane__date .remove').click()
		await expect(page.locator('[data-task-id="2"] .swimlane-card__due')).toHaveCount(0)

		// It persists after reload
		await page.reload()
		await expect(page.locator('[data-task-id="2"] .swimlane-card__due')).toHaveCount(0)
	})

	test('completing a task from the card removes it and persists after reload', async ({authenticatedPage: page}) => {
		await seedBoard()

		await page.goto('/tasks/overview')

		await page.locator('[data-task-id="1"]').getByTestId('task-done-checkbox').click()

		// Card disappears from the board of open tasks
		await expect(page.locator('[data-task-id="1"]')).toHaveCount(0)
		await expect(page.getByTestId('column-title-todo')).toContainText('2')

		// …and stays gone after a reload
		await page.reload()
		await expect(page.locator('[data-task-id="1"]')).toHaveCount(0)
		await expect(page.getByTestId('column-title-todo')).toContainText('2')
	})

	test('collapsing a lane hides its columns', async ({authenticatedPage: page}) => {
		const {projectA} = await seedBoard()

		await page.goto('/tasks/overview')

		const laneA = page.locator(`[data-cy="project-lane"][data-project-id="${projectA.id}"]`)
		await laneA.locator('.project-lane__label').click()
		await expect(laneA).toHaveClass(/is-collapsed/)
		await expect(laneA.getByTestId('swimlane-column-todo')).not.toBeVisible()

		await laneA.locator('.project-lane__label').click()
		await expect(laneA.getByTestId('swimlane-column-todo')).toBeVisible()
	})
})
