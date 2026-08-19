<template>
	<section
		class="task-detail-pane"
		:class="{'is-open': open}"
		data-cy="task-detail-pane"
		:aria-label="$t('task.overview.detailTitle')"
	>
		<div
			class="task-detail-pane__handle"
			@click="$emit('close')"
		/>
		<template v-if="task">
			<div class="task-detail-pane__header">
				<div class="task-detail-pane__project">
					<ColorBubble
						v-if="project && project.hexColor !== ''"
						:color="bubbleColor(project.hexColor)"
					/>
					<span v-if="project">{{ project.title }}</span>
				</div>
				<BaseButton
					class="task-detail-pane__close"
					data-cy="task-detail-pane-close"
					:aria-label="$t('task.overview.closeDetail')"
					@click="$emit('close')"
				>
					<span class="icon">
						<Icon icon="times" />
					</span>
				</BaseButton>
			</div>

			<h3
				class="task-detail-pane__title"
				data-cy="task-detail-pane-title"
			>
				{{ task.title }}
			</h3>
			<div class="task-detail-pane__meta">
				{{ task.identifier }} ·
				{{ formatDateSince(task.updated) }}
			</div>

			<div class="task-detail-pane__props">
				<div class="task-detail-pane__prop">
					<span class="task-detail-pane__prop-label">{{ $t('task.attributes.done') }}</span>
					<FancyCheckbox
						:disabled="updating"
						:model-value="task.done"
						@update:modelValue="toggleDone"
					/>
				</div>
				<div class="task-detail-pane__prop">
					<span class="task-detail-pane__prop-label">{{ $t('task.attributes.percentDone') }}</span>
					<PercentDoneSelect
						:disabled="updating"
						:model-value="task.percentDone"
						@update:modelValue="setPercentDone"
					/>
				</div>
				<div
					v-if="task.dueDate !== null && task.dueDate.getTime() > 0"
					class="task-detail-pane__prop"
				>
					<span class="task-detail-pane__prop-label">{{ $t('task.attributes.dueDate') }}</span>
					<span
						class="task-detail-pane__due"
						:class="{'is-overdue': isOverdue}"
					>
						{{ formatDateLong(task.dueDate) }}
					</span>
				</div>
				<div class="task-detail-pane__prop">
					<span class="task-detail-pane__prop-label">{{ $t('task.attributes.priority') }}</span>
					<PriorityLabel
						:priority="task.priority"
						:done="task.done"
						:show-all="true"
					/>
				</div>
				<div
					v-if="task.labels.length > 0"
					class="task-detail-pane__prop"
				>
					<span class="task-detail-pane__prop-label">{{ $t('task.attributes.labels') }}</span>
					<Labels :labels="task.labels" />
				</div>
				<div
					v-if="task.assignees.length > 0"
					class="task-detail-pane__prop"
				>
					<span class="task-detail-pane__prop-label">{{ $t('task.attributes.assignees') }}</span>
					<AssigneeList
						:assignees="task.assignees"
						:avatar-size="24"
					/>
				</div>
			</div>

			<template v-if="!isEditorContentEmpty(task.description)">
				<h4 class="task-detail-pane__section-title">
					{{ $t('task.attributes.description') }}
				</h4>
				<TipTap
					:model-value="task.description"
					:is-edit-enabled="false"
				/>
			</template>

			<div class="task-detail-pane__actions">
				<XButton
					variant="secondary"
					:to="{name: 'task.detail', params: {id: task.id}}"
				>
					{{ $t('task.overview.openTaskDetail') }}
				</XButton>
			</div>
		</template>
	</section>
</template>

<script lang="ts" setup>
import {computed, ref} from 'vue'

import BaseButton from '@/components/base/BaseButton.vue'
import ColorBubble from '@/components/misc/ColorBubble.vue'
import FancyCheckbox from '@/components/input/FancyCheckbox.vue'
import Labels from '@/components/tasks/partials/Labels.vue'
import AssigneeList from '@/components/tasks/partials/AssigneeList.vue'
import PercentDoneSelect from '@/components/tasks/partials/PercentDoneSelect.vue'
import PriorityLabel from '@/components/tasks/partials/PriorityLabel.vue'
import TipTap from '@/components/input/editor/TipTap.vue'
import XButton from '@/components/input/Button.vue'

import {formatDateLong, formatDateSince} from '@/helpers/time/formatDate'
import {isEditorContentEmpty} from '@/helpers/editorContentEmpty'
import {getHexColor} from '@/models/task'
import {useTaskStore} from '@/stores/tasks'
import {useProjectStore} from '@/stores/projects'
import {isTaskOverdue} from '@/composables/useSwimlaneTasks'

import type {ITask} from '@/modelTypes/ITask'

const props = defineProps<{
	task: ITask | null
	// Controls the mobile bottom sheet; on desktop the pane is always visible.
	open?: boolean
}>()

const emit = defineEmits<{
	'close': []
	'updated': [task: ITask]
}>()

const taskStore = useTaskStore()
const projectStore = useProjectStore()
const updating = ref(false)

const project = computed(() => props.task
	? projectStore.projects[props.task.projectId]
	: null)

function bubbleColor(hexColor: string): string {
	return getHexColor(hexColor) || hexColor
}

const isOverdue = computed(() => props.task ? isTaskOverdue(props.task, new Date()) : false)

async function save(changes: Partial<ITask>) {
	if (!props.task) return
	updating.value = true
	try {
		const updated = await taskStore.update({
			...props.task,
			...changes,
		})
		emit('updated', updated)
	} finally {
		updating.value = false
	}
}

function toggleDone(done: boolean) {
	save({done})
}

function setPercentDone(percentDone: number) {
	save({percentDone})
}
</script>

<style lang="scss" scoped>
.task-detail-pane {
	display: flex;
	flex-direction: column;
	gap: .75rem;
	background: var(--white);
	border-inline-start: 1px solid var(--border);
	padding: 1rem 1.1rem;
	overflow-y: auto;

	.task-detail-pane__handle {
		display: none;
	}

	@media screen and (width <= $tablet) {
		position: fixed;
		inset-inline: 0;
		inset-block-end: 0;
		block-size: 88vh;
		z-index: 40;
		border-inline-start: none;
		border-block-start: 1px solid var(--border);
		border-radius: var(--radius) var(--radius) 0 0;
		box-shadow: 0 -1rem 2.5rem hsla(var(--grey-500-hsl), .3);
		padding-block-start: .5rem;
		transform: translateY(105%);
		transition: transform 250ms ease;

		&.is-open {
			transform: none;
		}

		.task-detail-pane__handle {
			display: block;
			inline-size: 3rem;
			block-size: .3rem;
			border-radius: 999px;
			background: var(--grey-300);
			margin: 0 auto .5rem;
			cursor: pointer;
		}
	}
}

.task-detail-pane__header {
	display: flex;
	align-items: center;
	gap: .5rem;
}

.task-detail-pane__project {
	display: inline-flex;
	align-items: center;
	gap: .4rem;
	font-size: .75rem;
	font-weight: 600;
	color: var(--grey-600);
}

.task-detail-pane__close {
	margin-inline-start: auto;
	color: var(--grey-500);
}

.task-detail-pane__title {
	font-size: 1.1rem;
	font-weight: 700;
	line-height: 1.35;
	margin: 0;
}

.task-detail-pane__meta {
	font-size: .75rem;
	color: var(--grey-500);
}

.task-detail-pane__props {
	display: flex;
	flex-direction: column;
	border-block-start: 1px solid var(--border);
}

.task-detail-pane__prop {
	display: flex;
	align-items: center;
	gap: .75rem;
	padding: .55rem 0;
	border-block-end: 1px solid var(--border);
	font-size: .85rem;
	min-block-size: 2.4rem;
}

.task-detail-pane__prop-label {
	inline-size: 6rem;
	flex-shrink: 0;
	color: var(--grey-500);
	font-size: .75rem;
}

.task-detail-pane__due {
	&.is-overdue {
		color: var(--danger);
		font-weight: 600;
	}
}

.task-detail-pane__section-title {
	font-size: .7rem;
	text-transform: uppercase;
	letter-spacing: .04em;
	color: var(--grey-500);
	margin: .5rem 0 0;
}

.task-detail-pane__actions {
	margin-block-start: .5rem;
	display: flex;
	justify-content: flex-end;
}
</style>
