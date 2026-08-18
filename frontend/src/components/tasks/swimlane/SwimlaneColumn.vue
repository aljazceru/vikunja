<template>
	<div class="swimlane-column">
		<div class="swimlane-column__title mbe-2">
			{{ $t(translationKey) }}
			<span class="swimlane-column__count">
				{{ tasks.length }}
			</span>
		</div>
		<draggable
			class="swimlane-column__bucket"
			:item-key="(task: ITask) => `task-${task.id}`"
			:list="localTasks"
			:group="`lane-${projectId}`"
			:animation="150"
			item-tag="div"
			:component-data="{
				class: 'swimlane-column__cards',
			}"
			@change="onChange"
		>
			<template #item="{element: task}: {element: ITask}">
				<SwimlaneTaskCard
					:task="task"
					:selected="task.id === selectedTaskId"
					@open="t => $emit('open', t)"
					@updated="t => $emit('updated', t)"
				/>
			</template>
		</draggable>
		<div
			v-if="localTasks.length === 0"
			class="swimlane-column__empty"
		>
			{{ $t('task.overview.emptyColumn') }}
		</div>
	</div>
</template>

<script lang="ts" setup>
import {ref, watch} from 'vue'
import draggable from 'zhyswan-vuedraggable'

import SwimlaneTaskCard from './SwimlaneTaskCard.vue'
import {useTaskStore} from '@/stores/tasks'
import {columnOfTask, type SwimlaneColumnKey} from '@/composables/useSwimlaneTasks'

import type {ITask} from '@/modelTypes/ITask'

const props = defineProps<{
	projectId: number
	column: SwimlaneColumnKey
	translationKey: string
	tasks: ITask[]
	selectedTaskId?: ITask['id'] | null
}>()

const emit = defineEmits<{
	'open': [task: ITask]
	'updated': [task: ITask]
}>()

// The drag & drop list is owned locally so vuedraggable can reorder freely;
// it is rebuilt whenever the tasks prop changes.
const localTasks = ref<ITask[]>([])
watch(
	() => props.tasks,
	tasks => {
		localTasks.value = [...tasks]
	},
	{deep: true, immediate: true},
)

const taskStore = useTaskStore()

// percentDone values used when a card is dragged into a derived column.
const ENTER_PERCENT_DONE: Record<SwimlaneColumnKey, number> = {
	todo: 0,
	progress: 0.1,
}

async function onChange(event: {added?: {element: ITask}, removed?: {element: ITask}}) {
	const task = event.added?.element
	if (!task) return

	const newColumn = props.column
	if (columnOfTask(task) === newColumn) return

	const updated = await taskStore.update({
		...task,
		percentDone: ENTER_PERCENT_DONE[newColumn],
	})
	emit('updated', updated)
}
</script>

<style lang="scss" scoped>
.swimlane-column {
	display: flex;
	flex-direction: column;
	min-inline-size: 0;
}

.swimlane-column__title {
	display: flex;
	align-items: center;
	gap: .4rem;
	font-size: .7rem;
	font-weight: 700;
	text-transform: uppercase;
	letter-spacing: .04em;
	color: var(--grey-500);
	padding: 0 .1rem .4rem;

	// only visible on mobile, where there is no shared header row
	@media screen and (width >= $tablet) {
		display: none;
	}
}

.swimlane-column__count {
	font-size: .65rem;
	background: var(--grey-200);
	color: var(--grey-600);
	border-radius: 999px;
	padding: 0 .4rem;
}

.swimlane-column__bucket {
	flex: 1;
	background: var(--background);
	border: 1px solid var(--border-light);
	border-radius: var(--radius);
	padding: .4rem;
	min-block-size: 3rem;
	display: flex;
	flex-direction: column;
}

.swimlane-column__cards {
	display: flex;
	flex-direction: column;
	gap: .4rem;
}

.swimlane-column__empty {
	color: var(--grey-400);
	font-size: .75rem;
	text-align: center;
	padding: .75rem 0;
}
</style>
