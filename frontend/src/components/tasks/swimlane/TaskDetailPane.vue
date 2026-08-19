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
						@update:model-value="done => save({done})"
					/>
				</div>
				<div class="task-detail-pane__prop">
					<span class="task-detail-pane__prop-label">{{ $t('task.attributes.percentDone') }}</span>
					<PercentDoneSelect
						:disabled="updating"
						:model-value="task.percentDone"
						@update:model-value="percentDone => save({percentDone})"
					/>
				</div>
				<div
					class="task-detail-pane__prop"
					:class="{'is-overdue': isOverdue}"
				>
					<span class="task-detail-pane__prop-label">{{ $t('task.attributes.dueDate') }}</span>
					<div class="task-detail-pane__date">
						<Datepicker
							v-model="dueDate"
							:choose-date-label="$t('task.detail.chooseDueDate')"
							:disabled="updating"
							@closeOnChange="saveDueDate"
						/>
						<BaseButton
							v-if="task.dueDate !== null && task.dueDate.getTime() > 0"
							class="remove"
							:aria-label="$t('task.detail.removeDueDate')"
							@click="() => save({dueDate: null})"
						>
							<span class="icon is-small">
								<Icon icon="times" />
							</span>
						</BaseButton>
					</div>
				</div>
				<div class="task-detail-pane__prop">
					<span class="task-detail-pane__prop-label">{{ $t('task.attributes.priority') }}</span>
					<PrioritySelect
						:disabled="updating"
						:model-value="task.priority"
						@update:model-value="priority => save({priority})"
					/>
				</div>
				<div class="task-detail-pane__prop task-detail-pane__prop--block">
					<span class="task-detail-pane__prop-label">{{ $t('task.attributes.assignees') }}</span>
					<EditAssignees
						:model-value="task.assignees"
						:task-id="task.id"
						:project-id="task.projectId"
						:disabled="updating"
						:list-id="task.projectId"
						@update:modelValue="assignees => save({assignees})"
					/>
				</div>
				<div class="task-detail-pane__prop task-detail-pane__prop--block">
					<span class="task-detail-pane__prop-label">{{ $t('task.attributes.labels') }}</span>
					<EditLabels
						:model-value="task.labels"
						:task-id="task.id"
						:disabled="updating"
						@update:modelValue="labels => save({labels})"
					/>
				</div>
				<div class="task-detail-pane__prop task-detail-pane__prop--block">
					<span class="task-detail-pane__prop-label">{{ $t('task.attributes.reminders') }}</span>
					<Reminders
						v-model="reminders"
						:disabled="updating"
						:default-relative-to="remindersDefaultRelativeTo"
						@update:modelValue="reminders => save({reminders})"
					/>
				</div>
			</div>

			<h4 class="task-detail-pane__section-title">
				{{ $t('task.attributes.description') }}
			</h4>
			<Description
				:model-value="task"
				:attachment-upload="attachmentUpload"
				:can-write="!updating"
				@update:modelValue="t => $emit('updated', t)"
			/>

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
import {computed, ref, watch} from 'vue'

import BaseButton from '@/components/base/BaseButton.vue'
import ColorBubble from '@/components/misc/ColorBubble.vue'
import FancyCheckbox from '@/components/input/FancyCheckbox.vue'
import PercentDoneSelect from '@/components/tasks/partials/PercentDoneSelect.vue'
import PrioritySelect from '@/components/tasks/partials/PrioritySelect.vue'
import EditAssignees from '@/components/tasks/partials/EditAssignees.vue'
import EditLabels from '@/components/tasks/partials/EditLabels.vue'
import Reminders from '@/components/tasks/partials/Reminders.vue'
import Description from '@/components/tasks/partials/Description.vue'
import Datepicker from '@/components/input/Datepicker.vue'
import XButton from '@/components/input/Button.vue'

import {formatDateSince} from '@/helpers/time/formatDate'
import {getHexColor} from '@/models/task'
import {uploadFile} from '@/helpers/attachments'
import {useTaskStore} from '@/stores/tasks'
import {useProjectStore} from '@/stores/projects'
import {isTaskOverdue} from '@/composables/useSwimlaneTasks'

import type {ITask} from '@/modelTypes/ITask'
import type {ITaskReminder} from '@/modelTypes/ITaskReminder'

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

// Local copies for components that mutate a v-model before the save happens.
const dueDate = ref<Date | null>(null)
const reminders = ref<ITaskReminder[]>([])

watch(() => props.task, task => {
	dueDate.value = task?.dueDate && task.dueDate.getTime() > 0 ? task.dueDate : null
	reminders.value = task?.reminders ?? []
}, {immediate: true})

const isOverdue = computed(() => props.task ? isTaskOverdue(props.task, new Date()) : false)

const remindersDefaultRelativeTo = computed(() => 'due-date')

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

function saveDueDate() {
	if (!dueDate.value) return
	save({dueDate: dueDate.value})
}

async function attachmentUpload(file: File, onSuccess?: (url: string) => void) {
	if (!props.task) return []
	const uploaded = await uploadFile(props.task.id, file, onSuccess)
	return uploaded
}

function bubbleColor(hexColor: string): string {
	return getHexColor(hexColor) || hexColor
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
	max-block-size: 100vh;

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

	&--block {
		flex-wrap: wrap;

		.task-detail-pane__prop-label {
			flex-basis: 100%;
		}
	}
}

.task-detail-pane__prop-label {
	inline-size: 6rem;
	flex-shrink: 0;
	color: var(--grey-500);
	font-size: .75rem;

	&:has(+ .task-detail-pane__date) {
		color: var(--grey-500);
	}
}

.task-detail-pane__date {
	display: flex;
	align-items: center;
	gap: .35rem;
}

.task-detail-pane__prop.is-overdue .task-detail-pane__prop-label {
	color: var(--danger);
	font-weight: 600;
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
