<template>
	<div
		class="swimlane-card"
		:class="{
			'is-selected': selected,
			'is-done': task.done,
			'is-overdue': isOverdue,
		}"
		@click.exact="$emit('open', task)"
	>
		<div class="swimlane-card__top">
			<FancyCheckbox
				class="swimlane-card__check"
				:disabled="loading"
				:model-value="task.done"
				@update:modelValue="toggleDone"
				@click.stop
			/>
			<div class="swimlane-card__title">
				{{ task.title }}
			</div>
			<span class="swimlane-card__identifier">
				{{ task.identifier }}
			</span>
		</div>

		<ProgressBar
			v-if="task.percentDone > 0"
			class="swimlane-card__progress"
			:value="task.percentDone * 100"
		/>

		<div class="swimlane-card__meta">
			<Labels
				v-if="task.labels.length > 0"
				:labels="task.labels"
				class="swimlane-card__labels"
			/>
			<ChecklistSummary
				:task="task"
				class="swimlane-card__checklist"
			/>
			<span
				v-if="hasDueDate"
				v-tooltip="formatDateLong(task.dueDate)"
				class="swimlane-card__due"
				:class="{
					'is-overdue': isOverdue,
					'is-today': isToday,
				}"
			>
				<span class="icon">
					<Icon :icon="['far', 'calendar-alt']" />
				</span>
				<time :datetime="formatISO(task.dueDate)">
					{{ formatDisplayDate(task.dueDate) }}
				</time>
			</span>
			<span
				v-if="priority > PRIORITIES.UNSET"
				class="swimlane-card__priority"
				:class="{
					'is-high': priority >= PRIORITIES.HIGH,
					'is-medium': priority > PRIORITIES.UNSET && priority < PRIORITIES.HIGH,
				}"
			>
				<Icon icon="flag" />
			</span>
			<AssigneeList
				v-if="task.assignees.length > 0"
				:assignees="task.assignees"
				:avatar-size="20"
				:inline="true"
				class="swimlane-card__assignees"
			/>
		</div>
	</div>
</template>

<script lang="ts" setup>
import {computed, ref} from 'vue'

import FancyCheckbox from '@/components/input/FancyCheckbox.vue'
import Labels from '@/components/tasks/partials/Labels.vue'
import ChecklistSummary from '@/components/tasks/partials/ChecklistSummary.vue'
import AssigneeList from '@/components/tasks/partials/AssigneeList.vue'
import ProgressBar from '@/components/misc/ProgressBar.vue'

import {PRIORITIES} from '@/constants/priorities'
import {formatDateLong, formatDisplayDate, formatISO} from '@/helpers/time/formatDate'
import {useGlobalNow} from '@/composables/useGlobalNow'
import {useTaskStore} from '@/stores/tasks'
import {isTaskOverdue} from '@/composables/useSwimlaneTasks'

import type {ITask} from '@/modelTypes/ITask'

const props = withDefaults(defineProps<{
	task: ITask
	selected?: boolean
}>(), {
	selected: false,
})

const emit = defineEmits<{
	'open': [task: ITask]
	'updated': [task: ITask]
}>()

const taskStore = useTaskStore()
const loading = ref(false)

const priority = computed(() => props.task.priority)

const {now} = useGlobalNow()
const isOverdue = computed(() => isTaskOverdue(props.task, now.value))
const hasDueDate = computed(() =>
	props.task.dueDate !== null && props.task.dueDate.getTime() > 0)
const isToday = computed(() => {
	if (!hasDueDate.value) return false
	const due = props.task.dueDate as Date
	return due.toDateString() === now.value.toDateString()
})

async function toggleDone(checked: boolean) {
	loading.value = true
	try {
		const updated = await taskStore.update({
			...props.task,
			done: checked,
		})
		emit('updated', updated)
	} finally {
		loading.value = false
	}
}
</script>

<style lang="scss" scoped>
.swimlane-card {
	background: var(--white);
	border: 1px solid var(--border);
	border-radius: var(--radius);
	padding: .6rem .7rem;
	cursor: pointer;
	transition: border-color 100ms ease, box-shadow 100ms ease, transform 100ms ease;

	&:hover {
		border-color: var(--border-hover);
		box-shadow: var(--shadow-sm);
		transform: translateY(-1px);
	}

	&.is-selected {
		border-color: var(--primary);
		box-shadow: 0 0 0 1px var(--primary);
	}

	&.is-done {
		opacity: .5;

		.swimlane-card__title {
			text-decoration: line-through;
		}
	}
}

.swimlane-card__top {
	display: flex;
	align-items: flex-start;
	gap: .5rem;
}

.swimlane-card__check {
	flex-shrink: 0;
	margin-block-start: .1rem;
}

.swimlane-card__title {
	flex: 1;
	font-size: .9rem;
	line-height: 1.4;
	word-break: break-word;
}

.swimlane-card__identifier {
	flex-shrink: 0;
	font-size: .7rem;
	color: var(--grey-500);
	font-family: var(--family-monospace);
	padding-block-start: .15rem;
}

.swimlane-card__progress {
	margin-block-start: .5rem;
}

.swimlane-card__meta {
	display: flex;
	align-items: center;
	flex-wrap: wrap;
	gap: .4rem;
	margin-block-start: .55rem;
	min-block-size: 20px;
}

.swimlane-card__due {
	display: inline-flex;
	align-items: center;
	gap: .25rem;
	font-size: .75rem;
	color: var(--grey-600);

	&.is-today {
		color: var(--primary);
		font-weight: 600;
	}

	&.is-overdue {
		color: var(--danger);
		font-weight: 600;
	}
}

.swimlane-card__priority {
	font-size: .75rem;
	color: var(--grey-400);

	&.is-medium {
		color: var(--warning);
	}

	&.is-high {
		color: var(--danger);
	}
}

.swimlane-card__assignees {
	margin-inline-start: auto;
}
</style>
