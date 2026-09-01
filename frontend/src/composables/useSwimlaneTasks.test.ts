import {beforeEach, describe, expect, it, vi} from 'vitest'
import {createPinia, setActivePinia} from 'pinia'

import {
	MAX_TASKS,
	canWriteTasksIn,
	columnOfTask,
	isTaskOverdue,
	useSwimlaneTasks,
} from '@/composables/useSwimlaneTasks'

import type {ITask} from '@/modelTypes/ITask'

// Scripted pagination for the mocked task service: page number → number of
// tasks on that page plus the total-pages header the api would return.
const pagination = vi.hoisted(() => ({
	pages: {} as Record<number, {count: number, totalPages: number}>,
	requestedPages: [] as number[],
}))

vi.mock('vue-router', async (importOriginal) => {
	const actual = await importOriginal<typeof import('vue-router')>()
	return {
		...actual,
		useRouter: () => ({push: vi.fn()}),
		useRoute: () => ({fullPath: ''}),
	}
})

vi.mock('vue-i18n', async (importOriginal) => {
	const actual = await importOriginal<typeof import('vue-i18n')>()
	return {
		...actual,
		useI18n: () => ({t: (key: string) => key}),
	}
})

vi.mock('@/services/task', () => ({
	default: class {
		resultCount = 0
		totalPages = 0

		async getAll(_model: unknown, _params: unknown, page: number) {
			pagination.requestedPages.push(page)
			const scripted = pagination.pages[page]
			if (!scripted) throw new Error(`unexpected page request: ${page}`)
			this.totalPages = scripted.totalPages
			this.resultCount = scripted.count
			return Array.from({length: scripted.count}, (_, i) => ({id: page * 1000 + i}))
		}
	},
}))

function taskWith(fields: Partial<ITask> = {}): ITask {
	return {
		id: 1,
		title: 'Task',
		description: '',
		done: false,
		doneAt: null,
		deletedAt: null,
		priority: 0,
		labels: [],
		assignees: [],
		dueDate: null,
		startDate: null,
		endDate: null,
		repeatAfter: {amount: 0, type: 'seconds'},
		repeatFromCurrentDate: false,
		repeatMode: 0,
		reminders: [],
		parentTaskId: 0,
		hexColor: '',
		percentDone: 0,
		relatedTasks: {},
		attachments: [],
		coverImageAttachmentId: null,
		identifier: 'T-1',
		index: 1,
		isFavorite: false,
		subscription: null,
		position: 0,
		reactions: {},
		comments: [],
		createdBy: null,
		created: new Date(),
		updated: new Date(),
		projectId: 1,
		...fields,
	} as ITask
}

describe('columnOfTask', () => {
	it('puts tasks without progress in the todo column', () => {
		expect(columnOfTask(taskWith({percentDone: 0}))).toBe('todo')
	})

	it('puts tasks with partial progress in the progress column', () => {
		expect(columnOfTask(taskWith({percentDone: 0.1}))).toBe('progress')
		expect(columnOfTask(taskWith({percentDone: 0.5}))).toBe('progress')
		expect(columnOfTask(taskWith({percentDone: 0.9}))).toBe('progress')
	})

	it('treats fully-done-but-open tasks as todo', () => {
		// percentDone of 1 without done is an edge case; they stay out of the
		// progress column since there is nothing "in between" left.
		expect(columnOfTask(taskWith({percentDone: 1}))).toBe('todo')
	})
})

describe('isTaskOverdue', () => {
	const now = new Date('2025-06-10T12:00:00Z')

	it('marks tasks with a past due date as overdue', () => {
		expect(isTaskOverdue(taskWith({dueDate: new Date('2025-06-09T00:00:00Z')}), now)).toBe(true)
	})

	it('does not mark tasks due later as overdue', () => {
		expect(isTaskOverdue(taskWith({dueDate: new Date('2025-06-11T00:00:00Z')}), now)).toBe(false)
	})

	it('does not mark tasks without a due date as overdue', () => {
		expect(isTaskOverdue(taskWith({dueDate: null}), now)).toBe(false)
	})

	it('does not mark done tasks as overdue', () => {
		expect(isTaskOverdue(taskWith({done: true, dueDate: new Date('2025-06-09T00:00:00Z')}), now)).toBe(false)
	})
})

describe('constants', () => {
	it('caps the number of fetched tasks at a sane maximum', () => {
		expect(MAX_TASKS).toBeLessThanOrEqual(500)
		expect(MAX_TASKS).toBeGreaterThan(0)
	})
})

describe('canWriteTasksIn', () => {
	it('allows writes for projects with read/write or admin permission', () => {
		expect(canWriteTasksIn({maxPermission: 1, isArchived: false})).toBe(true)
		expect(canWriteTasksIn({maxPermission: 2, isArchived: false})).toBe(true)
	})

	it('denies writes for read-only, unknown or archived projects', () => {
		expect(canWriteTasksIn({maxPermission: 0, isArchived: false})).toBe(false)
		expect(canWriteTasksIn({maxPermission: null, isArchived: false})).toBe(false)
		expect(canWriteTasksIn({maxPermission: 2, isArchived: true})).toBe(false)
	})
})

describe('load', () => {
	beforeEach(() => {
		setActivePinia(createPinia())
		pagination.pages = {}
		pagination.requestedPages = []
	})

	function scriptPages(taskCount: number, perPage: number) {
		const totalPages = Math.ceil(taskCount / perPage)
		for (let page = 1; page <= totalPages; page++) {
			pagination.pages[page] = {
				count: Math.min(perPage, taskCount - (page - 1) * perPage),
				totalPages,
			}
		}
		return totalPages
	}

	it('reports the exact total when the cap stops pagination early', async () => {
		// 550 tasks do not fit under the 500-task cap; the total must come
		// from the final page's count instead of assuming it is full.
		scriptPages(550, 100)

		const {load, tasks, total} = useSwimlaneTasks()
		await load()

		expect(tasks.value).toHaveLength(MAX_TASKS)
		expect(total.value).toBe(550)
		// pages 1-5 for the board plus one request for the final page's count
		expect(pagination.requestedPages).toEqual([1, 2, 3, 4, 5, 6])
	})

	it('does not refetch when all pages fit under the cap', async () => {
		scriptPages(120, 100)

		const {load, tasks, total} = useSwimlaneTasks()
		await load()

		expect(tasks.value).toHaveLength(120)
		expect(total.value).toBe(120)
		expect(pagination.requestedPages).toEqual([1, 2])
	})

	it('does not refetch when the cap lands exactly on the last page', async () => {
		scriptPages(500, 100)

		const {load, tasks, total} = useSwimlaneTasks()
		await load()

		expect(tasks.value).toHaveLength(500)
		expect(total.value).toBe(500)
		expect(pagination.requestedPages).toEqual([1, 2, 3, 4, 5])
	})

	it('reports zero when nothing matches the open-task filter', async () => {
		pagination.pages[1] = {count: 0, totalPages: 0}

		const {load, tasks, total} = useSwimlaneTasks()
		await load()

		expect(tasks.value).toHaveLength(0)
		expect(total.value).toBe(0)
		expect(pagination.requestedPages).toEqual([1])
	})

	it('keeps a failed load in the error state instead of an empty board', async () => {
		// No scripted pages: every request rejects.
		pagination.pages = {}

		const {load, tasks, total, error} = useSwimlaneTasks()
		await load()

		expect(error.value).toBeInstanceOf(Error)
		expect(tasks.value).toHaveLength(0)
		expect(total.value).toBe(0)
	})

	it('clears a previous error after a successful reload', async () => {
		pagination.pages = {}
		const {load, error} = useSwimlaneTasks()
		await load()
		expect(error.value).not.toBeNull()

		scriptPages(10, 100)
		await load()
		expect(error.value).toBeNull()
	})
})
