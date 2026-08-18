import {computed, ref} from 'vue'

import TaskService from '@/services/task'
import TaskModel from '@/models/task'
import {useAuthStore} from '@/stores/auth'
import {useProjectStore} from '@/stores/projects'

import type {ITask} from '@/modelTypes/ITask'
import type {IProject} from '@/modelTypes/IProject'

export type SwimlaneColumnKey = 'todo' | 'progress'

// The minimal project info a lane needs — lets lanes work with projects from
// the store as well as unknown-project fallbacks.
export interface SwimlaneProject {
	id: IProject['id']
	title: string
	hexColor: string
}

export interface SwimlaneLane {
	project: SwimlaneProject
	tasks: ITask[]
	overdueCount: number
	nextDueDate: Date | null
}

// How many tasks to fetch per page and how many we're willing to render at most.
// The board is meant as an overview; anything beyond this should be filtered down.
const PAGE_SIZE = 100
export const MAX_TASKS = 500

export const SWIMLANE_COLUMNS: {key: SwimlaneColumnKey, translationKey: string}[] = [
	{key: 'todo', translationKey: 'task.overview.columns.todo'},
	{key: 'progress', translationKey: 'task.overview.columns.progress'},
]

/**
 * Maps a task to its swimlane column. There is no status field on tasks, so
 * columns are derived from percent_done: 0 means "to do", anything between
 * 0 and 1 exclusive means "in progress".
 */
export function columnOfTask(task: ITask): SwimlaneColumnKey {
	return task.percentDone > 0 && task.percentDone < 1
		? 'progress'
		: 'todo'
}

export function isTaskOverdue(task: ITask, now: Date): boolean {
	return !task.done &&
		task.dueDate !== null &&
		task.dueDate.getTime() > 0 &&
		task.dueDate.getTime() <= now.getTime()
}

export function useSwimlaneTasks() {
	const authStore = useAuthStore()
	const projectStore = useProjectStore()

	const isLoading = ref(false)
	const tasks = ref<ITask[]>([])
	const total = ref(0)

	async function load() {
		const taskService = new TaskService()
		isLoading.value = true
		try {
			const collected: ITask[] = []
			let page = 1
			do {
				const pageTasks = await taskService.getAll(new TaskModel(), {
					filter: 'done = false',
					filter_timezone: authStore.settings.timezone,
					sort_by: ['due_date', 'id'],
					order_by: ['asc', 'asc'],
					per_page: PAGE_SIZE,
				}, page)
				collected.push(...pageTasks)
				// The pagination headers describe the whole result set, not just this
				// page: reconstruct the total from the last page's count. A result
				// count of 0 means the whole filter matches nothing.
				total.value = taskService.resultCount === 0
					? 0
					: (taskService.totalPages - 1) * PAGE_SIZE + taskService.resultCount
				page++
			} while (page <= taskService.totalPages && collected.length < MAX_TASKS)
			tasks.value = collected
		} finally {
			isLoading.value = false
		}
	}

	const lanes = computed<SwimlaneLane[]>(() => {
		const byProject = new Map<IProject['id'], ITask[]>()
		for (const task of tasks.value) {
			const bucket = byProject.get(task.projectId) ?? []
			bucket.push(task)
			byProject.set(task.projectId, bucket)
		}

		const now = new Date()
		// Lanes follow the sidebar's project order; projects with tasks but not
		// in the store (shouldn't happen — they'd be unreadable) go last.
		const ordered: SwimlaneProject[] = [
			...projectStore.projectsArray.filter(p => byProject.has(p.id)),
			...[...byProject.keys()]
				.filter(id => !projectStore.projects[id])
				.map(id => ({id, title: `#${id}`, hexColor: '', position: Number.MAX_SAFE_INTEGER})),
		]

		return ordered.map(project => {
			const laneTasks = byProject.get(project.id) ?? []
			const overdueCount = laneTasks.filter(t => isTaskOverdue(t, now)).length
			const upcoming = laneTasks
				.filter(t => t.dueDate !== null && t.dueDate.getTime() > 0)
				.map(t => t.dueDate as Date)
				.sort((a, b) => a.getTime() - b.getTime())
			return {
				project,
				tasks: laneTasks,
				overdueCount,
				nextDueDate: upcoming[0] ?? null,
			}
		})
	})

	const overdueCount = computed(() =>
		tasks.value.reduce((sum, t) => sum + (isTaskOverdue(t, new Date()) ? 1 : 0), 0))

	function applyUpdate(updated: ITask) {
		const i = tasks.value.findIndex(t => t.id === updated.id)
		if (i === -1) return
		if (updated.done) {
			// Completed tasks leave the board of open tasks.
			tasks.value = [
				...tasks.value.slice(0, i),
				...tasks.value.slice(i + 1),
			]
			total.value = Math.max(0, total.value - 1)
			return
		}
		tasks.value = [
			...tasks.value.slice(0, i),
			updated,
			...tasks.value.slice(i + 1),
		]
	}

	return {
		isLoading,
		tasks,
		total,
		overdueCount,
		lanes,
		load,
		applyUpdate,
	}
}
