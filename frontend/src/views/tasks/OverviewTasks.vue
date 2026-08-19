<template>
	<div class="task-overview">
		<div class="task-overview__main">
			<div class="task-overview__header">
				<div class="task-overview__heading">
					<h1 class="task-overview__title">
						{{ $t('task.overview.title') }}
					</h1>
					<div
						v-if="!isLoading"
						class="task-overview__chips"
					>
						<span class="task-overview__chip">
							{{ $t('task.overview.taskCount', total) }}
						</span>
						<span
							v-if="overdueCount > 0"
							class="task-overview__chip is-warning"
						>
							{{ $t('task.overview.overdueCount', {count: overdueCount}) }}
						</span>
					</div>
				</div>
				<XButton
					variant="secondary"
					icon="arrows-rotate"
					:shadow="false"
					:loading="isLoading"
					@click="load"
				>
					{{ $t('task.overview.refresh') }}
				</XButton>
			</div>

			<Loading
				v-if="isLoading && lanes.length === 0"
				class="task-overview__loading"
			/>
			<SwimlaneBoard
				v-else-if="lanes.length > 0"
				:lanes="lanes"
				:tasks="tasks"
				:total="total"
				@select="selectTask"
				@updated="applyUpdate"
			/>
			<Message
				v-else
				class="task-overview__empty"
				variant="success"
			>
				{{ $t('task.overview.noTasks') }}
			</Message>

			<div
				v-if="selectedTask !== null"
				class="task-overview__scrim"
				@click="selectTask(null)"
			/>
		</div>

		<TaskDetailPane
			:class="{'is-empty': selectedTask === null}"
			:task="selectedTask"
			:open="selectedTask !== null"
			@close="selectTask(null)"
			@updated="onTaskUpdated"
		/>
	</div>
</template>

<script lang="ts" setup>
import {onMounted, ref} from 'vue'

import SwimlaneBoard from '@/components/tasks/swimlane/SwimlaneBoard.vue'
import TaskDetailPane from '@/components/tasks/swimlane/TaskDetailPane.vue'
import Loading from '@/components/misc/Loading.vue'
import Message from '@/components/misc/Message.vue'
import XButton from '@/components/input/Button.vue'

import {useSwimlaneTasks} from '@/composables/useSwimlaneTasks'

import type {ITask} from '@/modelTypes/ITask'

const {
	isLoading,
	tasks,
	total,
	overdueCount,
	lanes,
	load,
	applyUpdate,
} = useSwimlaneTasks()

const selectedTask = ref<ITask | null>(null)

onMounted(() => load())

function selectTask(task: ITask | null) {
	selectedTask.value = task
}

function onTaskUpdated(task: ITask) {
	applyUpdate(task)
	if (selectedTask.value?.id === task.id) {
		selectedTask.value = task.done ? null : task
	}
}
</script>

<style lang="scss" scoped>
// Focus look scoped to this page only: the accent switches to indigo for
// everything inside the overview while the rest of the app is untouched.
.task-overview {
	--primary-h: 240deg;
	--primary-s: 60%;
	--primary-l: 60%;
	--primary-hsl: var(--primary-h), var(--primary-s), var(--primary-l);
	--primary: hsla(var(--primary-h), var(--primary-s), var(--primary-l), var(--primary-a));
	--link: var(--primary);
	--switch-view-active-background: var(--primary);

	display: grid;
	grid-template-columns: minmax(0, 1fr) minmax(20rem, 22rem);
	align-items: start;
	block-size: 100%;
	background: var(--site-background);

	html.dark & {
		// Lighter indigo keeps sufficient contrast on dark surfaces
		--primary-l: 68%;
	}

	@media screen and (width <= $tablet) {
		display: block;
	}
}

.task-overview__main {
	display: flex;
	flex-direction: column;
	gap: .9rem;
	padding: 1rem 1.25rem 2rem;
	min-inline-size: 0;
}

.task-overview__header {
	display: flex;
	align-items: center;
	gap: 1rem;
}

.task-overview__heading {
	display: flex;
	align-items: baseline;
	flex-wrap: wrap;
	gap: .75rem;
	min-inline-size: 0;
}

.task-overview__title {
	font-size: 1.15rem;
	font-weight: 700;
	margin: 0;
}

.task-overview__chips {
	display: flex;
	align-items: center;
	gap: .4rem;
}

.task-overview__chip {
	font-size: .7rem;
	font-weight: 500;
	color: var(--grey-600);
	background: var(--grey-100);
	border-radius: 999px;
	padding: .15rem .6rem;

	&.is-warning {
		color: var(--danger);
		background: var(--danger-light);
		font-weight: 600;
	}
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
