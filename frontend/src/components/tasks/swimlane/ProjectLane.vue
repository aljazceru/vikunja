<template>
	<div
		class="project-lane"
		:class="{'is-collapsed': collapsed}"
	>
		<button
			class="project-lane__header"
			:aria-expanded="!collapsed"
			@click="$emit('toggleCollapse')"
		>
			<span class="project-lane__arrow icon">
				<Icon icon="chevron-down" />
			</span>
			<ColorBubble
				v-if="project.hexColor !== ''"
				:color="getProjectColor(project)"
			/>
			<span class="project-lane__title">
				{{ project.title }}
			</span>
			<span
				v-if="lane.overdueCount > 0"
				class="project-lane__overdue"
			>
				{{ $t('task.overview.laneOverdue', {count: lane.overdueCount}) }}
			</span>
			<span class="project-lane__count">
				{{ lane.tasks.length }}
			</span>
			<span
				v-if="lane.nextDueDate && !collapsed"
				class="project-lane__next"
			>
				{{ $t('task.overview.laneNext', {date: formatDisplayDate(lane.nextDueDate)}) }}
			</span>
		</button>
		<div
			v-if="!collapsed"
			class="project-lane__columns"
		>
			<SwimlaneColumn
				v-for="column in SWIMLANE_COLUMNS"
				:key="column.key"
				:project-id="project.id"
				:column="column.key"
				:translation-key="column.translationKey"
				:tasks="lane.tasks.filter(t => columnOfTask(t) === column.key)"
				:selected-task-id="selectedTaskId"
				@open="t => $emit('open', t)"
				@updated="t => $emit('updated', t)"
			/>
		</div>
		<div
			v-else-if="lane.nextDueDate"
			class="project-lane__next project-lane__next--collapsed"
		>
			{{ $t('task.overview.laneNext', {date: formatDisplayDate(lane.nextDueDate)}) }}
		</div>
	</div>
</template>

<script lang="ts" setup>
import SwimlaneColumn from './SwimlaneColumn.vue'
import ColorBubble from '@/components/misc/ColorBubble.vue'

import {columnOfTask, SWIMLANE_COLUMNS, type SwimlaneLane, type SwimlaneProject} from '@/composables/useSwimlaneTasks'
import {formatDisplayDate} from '@/helpers/time/formatDate'
import {getHexColor} from '@/models/task'

import type {ITask} from '@/modelTypes/ITask'

defineProps<{
	lane: SwimlaneLane
	project: SwimlaneProject
	collapsed: boolean
	selectedTaskId?: ITask['id'] | null
}>()

defineEmits<{
	'toggleCollapse': []
	'open': [task: ITask]
	'updated': [task: ITask]
}>()

function getProjectColor(project: SwimlaneProject): string {
	return getHexColor(project.hexColor) || project.hexColor
}
</script>

<style lang="scss" scoped>
.project-lane {
	display: flex;
	flex-direction: column;
	gap: .5rem;
	min-inline-size: 0;
}

.project-lane__header {
	display: flex;
	align-items: center;
	gap: .5rem;
	inline-size: 100%;
	background: var(--white);
	border: 1px solid var(--border-light);
	border-radius: var(--radius);
	padding: .55rem .7rem;
	font: inherit;
	text-align: start;
	cursor: pointer;
	transition: border-color 100ms ease;

	&:hover {
		border-color: var(--border-hover);
	}
}

.project-lane__arrow {
	color: var(--grey-400);
	font-size: .75rem;
	transition: transform 100ms ease;

	.is-collapsed & {
		transform: rotate(-90deg);
	}
}

.project-lane__title {
	font-weight: 600;
	font-size: .9rem;
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;
}

.project-lane__count {
	font-size: .7rem;
	background: var(--grey-200);
	color: var(--grey-600);
	border-radius: 999px;
	padding: .05rem .5rem;
}

.project-lane__overdue {
	font-size: .7rem;
	font-weight: 600;
	color: var(--danger);
	background: var(--danger-light);
	border-radius: 999px;
	padding: .05rem .5rem;
}

.project-lane__next {
	margin-inline-start: auto;
	font-size: .7rem;
	color: var(--grey-500);
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;

	&--collapsed {
		padding: 0 .7rem;
	}
}

.project-lane__columns {
	display: grid;
	grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
	gap: .6rem;

	@media screen and (width <= $tablet) {
		// On mobile the two columns become a swipeable row, like the classic
		// mobile kanban pattern (one column fills most of the viewport).
		display: flex;
		gap: .6rem;
		overflow-x: auto;
		scroll-snap-type: x mandatory;
		padding-block-end: .4rem;

		> * {
			min-inline-size: min(85vw, 22rem);
			scroll-snap-align: start;
		}
	}
}
</style>
