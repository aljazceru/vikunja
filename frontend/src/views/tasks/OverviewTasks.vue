<template>
	<div class="task-overview">
		<div class="task-overview__main">
			<div class="task-overview__header">
				<div>
					<h1 class="task-overview__title">
						{{ $t('task.overview.title') }}
					</h1>
					<p
						v-if="!isLoading"
						class="task-overview__subtitle"
					>
						<i18n-t
							keypath="task.overview.subtitle"
							tag="span"
						>
							<template #total>
								{{ total }}
							</template>
							<template #projects>
								{{ lanes.length }}
							</template>
						</i18n-t>
						<span
							v-if="overdueCount > 0"
							class="task-overview__overdue"
						>
							{{ $t('task.overview.overdueCount', {count: overdueCount}) }}
						</span>
					</p>
				</div>
				<XButton
					variant="secondary"
					icon="arrows-rotate"
					:loading="isLoading"
					@click="reload"
				>
					{{ $t('task.overview.refresh') }}
				</XButton>
			</div>

			<Loading
				v-if="isLoading && lanes.length === 0"
				class="task-overview__loading"
			/>
			<template v-else-if="lanes.length > 0">
				<SwimlaneBoard
					ref="board"
					@select="selectTask"
				/>
			</template>
			<Message
				v-else
				class="task-overview__empty"
				variant="success"
			>
				{{ $t('task.overview.noTasks') }}
			</Message>

			<div
				v-if="sheetOpen"
				class="task-overview__scrim"
				@click="selectTask(null)"
			/>
		</div>

		<TaskDetailPane
			:class="{'is-empty': selectedTask === null}"
			:task="selectedTask"
			:open="sheetOpen"
			@close="selectTask(null)"
			@updated="onTaskUpdated"
		/>
	</div>
</template>

<script lang="ts" setup>
import {computed, ref} from 'vue'

import SwimlaneBoard from '@/components/tasks/swimlane/SwimlaneBoard.vue'
import TaskDetailPane from '@/components/tasks/swimlane/TaskDetailPane.vue'
import Loading from '@/components/misc/Loading.vue'
import Message from '@/components/misc/Message.vue'
import XButton from '@/components/input/Button.vue'

import type {ITask} from '@/modelTypes/ITask'

const board = ref<InstanceType<typeof SwimlaneBoard> | null>(null)

const isLoading = computed(() => board.value?.isLoading ?? true)
const total = computed(() => board.value?.total ?? 0)
const overdueCount = computed(() => board.value?.overdueCount ?? 0)
const lanes = computed(() => board.value?.lanes ?? [])

const selectedTask = ref<ITask | null>(null)
// On mobile the pane is a bottom sheet that only appears when a task was
// picked; on desktop the pane is always mounted (empty state when nothing is
// selected).
const sheetOpen = computed(() => selectedTask.value !== null)

function reload() {
	board.value?.reload()
}

function selectTask(task: ITask | null) {
	selectedTask.value = task
}

function onTaskUpdated(task: ITask) {
	if (selectedTask.value?.id === task.id) {
		selectedTask.value = task.done ? null : task
	}
}
</script>

<style lang="scss" scoped>
.task-overview {
	display: grid;
	grid-template-columns: minmax(0, 1fr) minmax(20rem, 22rem);
	gap: 0;
	align-items: start;
	min-block-size: 100%;

	@media screen and (width <= $tablet) {
		display: block;
	}
}

.task-overview__main {
	display: flex;
	flex-direction: column;
	gap: .75rem;
	padding: .75rem 1rem 2rem;
	min-inline-size: 0;
}

.task-overview__header {
	display: flex;
	align-items: flex-start;
	gap: 1rem;
}

.task-overview__title {
	font-size: 1.35rem;
	font-weight: 700;
	margin: 0;
}

.task-overview__subtitle {
	color: var(--grey-600);
	font-size: .85rem;
	margin: .15rem 0 0;
}

.task-overview__overdue {
	color: var(--danger);
	font-weight: 600;
	margin-inline-start: .5rem;
}

.task-overview__loading,
.task-overview__empty {
	margin-block-start: 2rem;
}

.task-overview__scrim {
	display: none;

	@media screen and (width <= $tablet) {
		display: block;
		position: fixed;
		inset: 0;
		z-index: 30;
		background: rgba(0, 0, 0, .45);
	}
}
</style>
