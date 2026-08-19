<template>
	<div
		class="swimlane-card"
		:class="{
			'is-selected': selected,
			'is-done': task.done,
		}"
		data-cy="swimlane-card"
		:data-task-id="task.id"
		@click.exact="$emit('open', task)"
	>
		<div class="swimlane-card__top">
			<BaseButton
				class="swimlane-card__check"
				:class="{'is-checked': task.done}"
				data-cy="task-done-checkbox"
				:aria-label="task.done
					? $t('task.overview.markAsUndone')
					: $t('task.overview.markAsDone')"
				:disabled="loading"
				@click.stop="toggleDone(!task.done)"
			>
				<span class="icon">
					<Icon icon="check" />
				</span>
			</BaseButton>
			<div class="swimlane-card__title">
				{{ task.title }}
			</div>
		</div>

		<div class="swimlane-card__meta">
			<span
				v-if="task.identifier !== ''"
				class="swimlane-card__identifier"
			>
				{{ task.identifier }}
			</span>
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

import BaseButton from '@/components/base/BaseButton.vue'
import Labels from '@/components/tasks/partials/Labels.vue'
import ChecklistSummary from '@/components/tasks/partials/ChecklistSummary.vue'
import AssigneeList from '@/components/tasks/partials/AssigneeList.vue'

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

async function toggleDone(done: boolean) {
	loading.value = true
	try {
		const updated = await taskStore.update({
			...props.task,
			done,
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
	border-radius: .625rem;
	padding: .65rem .7rem;
	cursor: pointer;
	transition: border-color 100ms ease, box-shadow 100ms ease, transform 100ms ease;

	&:hover {
		border-color: var(--border-hover);
		box-shadow: hsla(var(--grey-500-hsl), .12) 0 4px 12px;
		transform: translateY(-1px);
	}

	&.is-selected {
		border-color: var(--primary);
		box-shadow: 0 0 0 1px var(--primary);

		&:hover {
			box-shadow: hsla(var(--primary-hsl), .25) 0 4px 12px;
		}
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
	gap: .55rem;
}

.swimlane-card__check {
	flex-shrink: 0;
	inline-size: 1rem;
	block-size: 1rem;
	border-radius: 50%;
	border: 1.5px solid var(--grey-300);
	color: transparent;
	padding: 0;
	margin-block-start: .1rem;
	display: grid;
	place-items: center;
	transition: border-color 100ms ease, background-color 100ms ease;

	&:hover {
		border-color: var(--primary);
	}

	.icon {
		font-size: .55rem;
	}

	&.is-checked {
		background: var(--primary);
		border-color: var(--primary);
		color: var(--white);
	}
}

.swimlane-card__title {
	flex: 1;
	font-size: .875rem;
	line-height: 1.45;
	word-break: break-word;
}

.swimlane-card__meta {
	display: flex;
	align-items: center;
	flex-wrap: wrap;
	gap: .45rem;
	margin-block-start: .5rem;
	padding-inline-start: 1.55rem;
	min-block-size: 1.25rem;

	@media screen and (width <= $tablet) {
		padding-inline-start: 0;
	}
}

.swimlane-card__identifier {
	font-size: .68rem;
	color: var(--grey-400);
}

.swimlane-card__labels,
.swimlane-card__checklist {
	font-size: .7rem;
}

.swimlane-card__due {
	display: inline-flex;
	align-items: center;
	gap: .25rem;
	font-size: .7rem;
	font-weight: 500;
	color: var(--grey-500);
	margin-inline-start: auto;

	&.is-today {
		color: var(--primary);
		font-weight: 600;
	}

	&.is-overdue {
		color: var(--danger-text);
		font-weight: 600;
	}
}

.swimlane-card__priority {
	font-size: .7rem;
	color: var(--grey-300);

	&.is-medium {
		color: var(--warning);
	}

	&.is-high {
		color: var(--danger);
	}
}

.swimlane-card__assignees {
	margin-inline-start: auto;

	// when a due chip or priority is present, keep assignees next to them
	.swimlane-card__due ~ &,
	.swimlane-card__priority ~ & {
		margin-inline-start: 0;
	}
}
</style>
