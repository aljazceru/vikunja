<template>
	<div
		class="swimlane-column"
		:data-cy="`swimlane-column-${column}`"
	>
		<div class="swimlane-column__title">
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
	display: none;

	// Only visible on mobile, where the shared desktop header row is hidden
	@media screen and (width <= $tablet) {
		display: flex;
		align-items: center;
		gap: .4rem;
		font-size: .68rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: .04em;
		color: var(--grey-500);
		padding: 0 .15rem .4rem;
	}
}

.swimlane-column__count {
	font-size: .62rem;
	background: var(--grey-100);
	color: var(--grey-600);
	border-radius: 999px;
	padding: .05rem .45rem;
}

.swimlane-column__bucket {
	flex: 1;
	background: var(--grey-50);
	border: 1px solid var(--grey-100);
	border-radius: .625rem;
	padding: .45rem;
	min-block-size: 3rem;
}

.swimlane-column__cards {
	display: flex;
	flex-direction: column;
	gap: .45rem;
}

.swimlane-column__empty {
	color: var(--grey-300);
	font-size: .75rem;
	text-align: center;
	padding: .9rem 0 .6rem;
}
</style>
