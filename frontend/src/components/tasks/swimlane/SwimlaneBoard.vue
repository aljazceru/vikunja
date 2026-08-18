<template>
	<div class="swimlane-board">
		<div class="swimlane-board__header">
			<div class="swimlane-board__corner">
				{{ $t('task.overview.projects', {count: lanes.length}) }}
			</div>
			<div
				v-for="column in SWIMLANE_COLUMNS"
				:key="column.key"
				class="swimlane-board__column-title"
			>
				{{ $t(column.translationKey) }}
				<span class="swimlane-board__column-count">
					{{ tasksInColumn(column.key) }}
				</span>
			</div>
		</div>

		<ProjectLane
			v-for="lane in lanes"
			:key="lane.project.id"
			:lane="lane"
			:project="lane.project"
			:collapsed="collapsedProjects.has(lane.project.id)"
			:selected-task-id="selectedTaskId"
			@toggle-collapse="() => toggleCollapse(lane.project.id)"
			@open="openTask"
			@updated="onTaskUpdated"
		/>

		<p
			v-if="total > tasks.length"
			class="swimlane-board__notice"
		>
			{{ $t('task.overview.showingOf', {shown: tasks.length, total}) }}
		</p>
	</div>
</template>

<script lang="ts" setup>
import {onMounted, ref, watch} from 'vue'

import ProjectLane from './ProjectLane.vue'

import {
	SWIMLANE_COLUMNS,
	columnOfTask,
	useSwimlaneTasks,
	type SwimlaneColumnKey,
} from '@/composables/useSwimlaneTasks'

import type {ITask} from '@/modelTypes/ITask'
import type {IProject} from '@/modelTypes/IProject'

const emit = defineEmits<{
	'select': [task: ITask | null]
}>()

const {
	isLoading,
	tasks,
	total,
	overdueCount,
	lanes,
	load,
	applyUpdate,
} = useSwimlaneTasks()

const selectedTaskId = ref<ITask['id'] | null>(null)

// Lane collapse state persists per project across sessions.
const STORAGE_KEY = 'swimlane-overview-collapsed-projects'
const collapsedProjects = ref<Set<IProject['id']>>(new Set())

function restoreCollapsed() {
	try {
		const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) || '[]')
		collapsedProjects.value = new Set(Array.isArray(stored) ? stored : [])
	} catch {
		collapsedProjects.value = new Set()
	}
}

function toggleCollapse(projectId: IProject['id']) {
	const next = new Set(collapsedProjects.value)
	if (next.has(projectId)) {
		next.delete(projectId)
	} else {
		next.add(projectId)
	}
	collapsedProjects.value = next
	localStorage.setItem(STORAGE_KEY, JSON.stringify([...next]))
}

function tasksInColumn(column: SwimlaneColumnKey): number {
	return tasks.value.filter(t => columnOfTask(t) === column).length
}

function openTask(task: ITask) {
	selectedTaskId.value = task.id
	emit('select', task)
}

function onTaskUpdated(task: ITask) {
	applyUpdate(task)
	if (task.id === selectedTaskId.value && task.done) {
		selectedTaskId.value = null
		emit('select', null)
	}
}

// Keep the selected task in sync with updates (e.g. after a drag between
// columns changed its percent done).
watch(tasks, () => {
	if (selectedTaskId.value === null) return
	const stillThere = tasks.value.some(t => t.id === selectedTaskId.value)
	if (!stillThere) {
		selectedTaskId.value = null
		emit('select', null)
	}
})

restoreCollapsed()
onMounted(() => load())

defineExpose({
	isLoading,
	lanes,
	overdueCount,
	total,
	reload: load,
})
</script>

<style lang="scss" scoped>
.swimlane-board {
	display: flex;
	flex-direction: column;
	gap: .625rem;
	min-inline-size: 0;
}

.swimlane-board__header {
	display: grid;
	grid-template-columns: 13rem repeat(2, minmax(0, 1fr));
	gap: .625rem;
	position: sticky;
	inset-block-start: 0;
	z-index: 3;
	padding-block-end: .35rem;
	background: var(--site-background);

	@media screen and (width <= $tablet) {
		display: none;
	}
}

.swimlane-board__corner {
	font-size: .68rem;
	font-weight: 600;
	text-transform: uppercase;
	letter-spacing: .04em;
	color: var(--grey-400);
	padding: .5rem .15rem;
}

.swimlane-board__column-title {
	display: flex;
	align-items: center;
	gap: .45rem;
	font-size: .68rem;
	font-weight: 700;
	text-transform: uppercase;
	letter-spacing: .04em;
	color: var(--grey-500);
	padding: .5rem .6rem;
}

.swimlane-board__column-count {
	margin-inline-start: auto;
	font-size: .62rem;
	font-weight: 600;
	color: var(--grey-400);
}

.swimlane-board__notice {
	font-size: .75rem;
	color: var(--grey-500);
	padding: .5rem .15rem 0;
}
</style>
