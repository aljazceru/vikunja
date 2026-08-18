import {describe, expect, it} from 'vitest'

import {
	MAX_TASKS,
	columnOfTask,
	isTaskOverdue,
} from '@/composables/useSwimlaneTasks'

import type {ITask} from '@/modelTypes/ITask'

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
