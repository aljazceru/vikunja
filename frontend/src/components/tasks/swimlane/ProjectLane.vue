<template>
	<div
		class="project-lane"
		:class="{'is-collapsed': collapsed}"
		data-cy="project-lane"
		:data-project-id="project.id"
	>
		<button
			class="project-lane__label"
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
			<span
				class="project-lane__title"
				data-cy="lane-title"
			>
				{{ project.title }}
			</span>
			<span class="project-lane__count">
				{{ lane.tasks.length }}
			</span>
			<span
				v-if="lane.overdueCount > 0"
				class="project-lane__overdue"
				data-cy="lane-overdue"
			>
				{{ $t('task.overview.laneOverdue', {count: lane.overdueCount}) }}
			</span>
			<span
				v-if="lane.nextDueDate"
				class="project-lane__next"
				:class="{'is-late': lane.overdueCount > 0}"
			>
				{{ $t('task.overview.laneNext', {date: formatDisplayDate(lane.nextDueDate)}) }}
			</span>
		</button>
		<div class="project-lane__columns">
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
	display: grid;
	grid-template-columns: 13rem repeat(2, minmax(0, 1fr));
	gap: .625rem;
	align-items: start;

	&.is-collapsed {
		grid-template-columns: 13rem;

		.project-lane__columns {
			display: none;
		}

		.project-lane__arrow {
			transform: rotate(-90deg);
		}
	}

	@media screen and (width <= $tablet) {
		display: block;
	}
}

.project-lane__label {
	display: flex;
	align-items: center;
	flex-wrap: wrap;
	gap: .5rem;
	inline-size: 100%;
	background: var(--white);
	border: 1px solid var(--border);
	border-radius: .625rem;
	padding: .7rem .75rem;
	font: inherit;
	text-align: start;
	color: var(--text);
	cursor: pointer;
	transition: border-color 100ms ease;

	&:hover {
		border-color: var(--border-hover);
	}

	@media screen and (width <= $tablet) {
		position: sticky;
		inset-block-start: 0;
		z-index: 2;
		margin-block-end: .5rem;
	}
}

.project-lane__arrow {
	color: var(--grey-400);
	font-size: .75rem;
	transition: transform 100ms ease;
}

.project-lane__title {
	font-weight: 600;
	font-size: .85rem;
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;
	max-inline-size: 100%;
}

.project-lane__count {
	margin-inline-start: auto;
	font-size: .7rem;
	font-weight: 600;
	color: var(--grey-500);
	background: var(--grey-100);
	border-radius: 999px;
	padding: .1rem .5rem;
}

.project-lane__overdue {
	font-size: .7rem;
	font-weight: 600;
	// Danger text on a faint translucent danger tint. --danger-text adapts
	// per color scheme; the alpha tint reads as a soft red on light surfaces
	// and a muted dark red on dark ones, instead of light-mode pink in dark.
	color: var(--danger-text);
	background: hsla(var(--danger-h), var(--danger-s), var(--danger-l), .12);
	border-radius: 999px;
	padding: .1rem .5rem;
}

.project-lane__next {
	flex-basis: 100%;
	font-size: .7rem;
	color: var(--grey-500);
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;

	&.is-late {
		color: var(--danger-text);
		font-weight: 600;
	}
}

.project-lane__columns {
	display: contents;

	@media screen and (width <= $tablet) {
		// Mobile kanban pattern: columns become a swipeable row under the
		// sticky lane header.
		display: flex;
		gap: .625rem;
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
