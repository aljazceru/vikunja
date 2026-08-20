<template>
	<ProjectWrapper
		class="project-list"
		:is-loading-project="isLoadingProject"
		:project-id="projectId"
		:view-id
	>
		<template #header>
			<div class="filter-container">
				<SortPopup
					v-model="sortByParam"
				/>
				<FilterPopup
					v-if="!isSavedFilter(project)"
					v-model="params"
					:view-id="viewId"
					:project-id="projectId"
					@update:modelValue="loadTasks()"
				/>
				<SubprojectsToggle
					v-if="!isSavedFilter(project)"
					:project-id="projectId"
				/>
			</div>
		</template>

		<template #default>
			<div
				:class="{ 'is-loading': loading }"
				class="loader-container is-max-width-desktop list-view"
			>
				<Card
					:padding="false"
					:has-content="false"
					class="has-overflow"
				>
					<AddTask
						v-if="!project?.isArchived && canWrite"
						ref="addTaskRef"
						class="list-view__add-task d-print-none"
						:default-position="firstNewPosition"
						@taskAdded="updateTaskList"
					/>

					<Nothing v-if="ctaVisible && tasks.length === 0 && !loading">
						{{ $t('project.list.empty') }}
						<ButtonLink
							v-if="project?.id > 0 && canWrite"
							@click="focusNewTaskInput()"
						>
							{{ $t('project.list.newTaskCta') }}
						</ButtonLink>
					</Nothing>

					<template v-if="isGroupedByProject && tasks.length > 0">
						<div
							v-for="group in groupedTasks"
							:key="group.project?.id ?? 'unknown'"
							class="project-section"
						>
							<div
								class="project-section__header"
								role="button"
								tabindex="0"
								@click="toggleSection(group.project?.id)"
								@keyup.enter="toggleSection(group.project?.id)"
							>
								<Icon
									:icon="isSectionCollapsed(group.project?.id) ? 'chevron-right' : 'chevron-down'"
									class="project-section__chevron"
								/>
								<ColorBubble
									v-if="group.project?.hexColor"
									:color="group.project.hexColor"
								/>
								<span class="project-section__title">{{ group.project?.title ?? $t('project.noDescriptionAvailable') }}</span>
								<span class="project-section__count">{{ group.tasks.length }}</span>
							</div>
							<ul
								v-if="!isSectionCollapsed(group.project?.id)"
								class="tasks project-section__tasks"
							>
								<li
									v-for="t in group.tasks"
									:key="t.id"
								>
									<SingleTaskInProject
										:show-list-color="false"
										:can-mark-as-done="canWrite || isPseudoProject"
										:the-task="t"
										:all-tasks="allTasks"
										@taskUpdated="updateTasks"
									/>
								</li>
							</ul>
						</div>
					</template>
					<draggable
						v-else-if="tasks && tasks.length > 0"
						v-model="tasks"
						:group="{name: 'tasks', put: false}"
						:disabled="!canDragTasks || !isPositionSorting"
						item-key="id"
						tag="ul"
						:component-data="{
							class: {
								tasks: true,
								'dragging-disabled': !canDragTasks || !isPositionSorting
							},
							type: 'transition-group'
						}"
						:animation="100"
						:handle="dragHandle"
						:delay-on-touch-only="!isTouchDevice"
						:delay="isTouchDevice ? 0 : 1000"
						ghost-class="task-ghost"
						@start="handleDragStart"
						@end="saveTaskPosition"
					>
						<template #item="{element: t, index}">
							<SingleTaskInProject
								:ref="(el) => setTaskRef(el, index)"
								:show-list-color="false"
								:can-mark-as-done="canWrite || isPseudoProject"
								:the-task="t"
								:all-tasks="allTasks"
								@taskUpdated="updateTasks"
							>
								<span
									v-if="canDragTasks && isPositionSorting"
									class="icon handle"
								>
									<Icon icon="grip-lines" />
								</span>
							</SingleTaskInProject>
						</template>
					</draggable>

					<Pagination
						:total-pages="totalPages"
						:current-page="currentPage"
					/>
				</Card>
			</div>
		</template>
	</ProjectWrapper>
</template>


<script setup lang="ts">
import {ref, computed, nextTick, onMounted, onBeforeUnmount, watch, toRef} from 'vue'
import draggable from 'zhyswan-vuedraggable'

import ProjectWrapper from '@/components/project/ProjectWrapper.vue'
import ButtonLink from '@/components/misc/ButtonLink.vue'
import AddTask from '@/components/tasks/AddTask.vue'
import SingleTaskInProject from '@/components/tasks/partials/SingleTaskInProject.vue'
import FilterPopup from '@/components/project/partials/FilterPopup.vue'
import Nothing from '@/components/misc/Nothing.vue'
import Pagination from '@/components/misc/Pagination.vue'
import SortPopup from '@/components/project/partials/SortPopup.vue'
import SubprojectsToggle from '@/components/project/partials/SubprojectsToggle.vue'
import ColorBubble from '@/components/misc/ColorBubble.vue'

import {useTaskList} from '@/composables/useTaskList'
import {useTaskDragToProject} from '@/composables/useTaskDragToProject'
import {shouldShowTaskInListView} from '@/composables/useTaskListFiltering'
import {PERMISSIONS as Permissions} from '@/constants/permissions'
import {calculateItemPosition} from '@/helpers/calculateItemPosition'
import type {ITask} from '@/modelTypes/ITask'
import {isSavedFilter, useSavedFilter} from '@/services/savedFilter'

import {useBaseStore} from '@/stores/base'
import {useTaskStore} from '@/stores/tasks'
import {useAuthStore} from '@/stores/auth'
import {useProjectStore} from '@/stores/projects'

import type {IProject} from '@/modelTypes/IProject'
import type {IProjectView} from '@/modelTypes/IProjectView'
import TaskPositionService from '@/services/taskPosition'
import TaskPositionModel from '@/models/taskPosition'

const props = defineProps<{
        isLoadingProject: boolean,
        projectId: IProject['id'],
        viewId: IProjectView['id'],
}>()

const projectId = toRef(props, 'projectId')

defineOptions({name: 'List'})

const ctaVisible = ref(false)

const drag = ref(false)

const {
	tasks: allTasks,
	loading,
	totalPages,
	currentPage,
	loadTasks,
	params,
	sortByParam,
} = useTaskList(
	() => projectId.value,
	() => props.viewId,
	{position: 'asc'},
	() => projectId.value === -1
		? ['comment_count', 'is_unread']
		: ['subtasks', 'comment_count', 'is_unread'],
)

const taskPositionService = ref(new TaskPositionService())

// Saved filter composable for accessing filter data
const _savedFilter = useSavedFilter(() => isSavedFilter({id: projectId.value}) ? projectId.value : undefined).filter

const tasks = ref<ITask[]>([])
watch(
	allTasks,
	() => {
		const isFiltered = isSavedFilter({id: projectId.value})
		tasks.value = ([...allTasks.value]).filter(t => shouldShowTaskInListView(t, allTasks.value, isFiltered))
	},
)

const isPositionSorting = computed(() => 'position' in sortByParam.value)

const firstNewPosition = computed(() => {
	if (tasks.value.length === 0) {
		return 0
	}

	return calculateItemPosition(null, tasks.value[0].position)
})

const baseStore = useBaseStore()
const taskStore = useTaskStore()
const {handleTaskDropToProject} = useTaskDragToProject()
const project = computed(() => baseStore.currentProject)

const canWrite = computed(() => {
	return project.value?.maxPermission > Permissions.READ && project.value?.id > 0
})

const isPseudoProject = computed(() => (project.value && isSavedFilter(project.value)) || project.value?.id === -1)

onMounted(async () => {
	await nextTick()
	ctaVisible.value = true
})

const canDragTasks = computed(() => canWrite.value || isSavedFilter(project.value))

const authStore = useAuthStore()
const projectStore = useProjectStore()

// When subproject tasks are included, group the flat task list into one
// collapsible section per source project (parent first, then descendants by
// title) — matches Todoist's sectioned list.
const isGroupedByProject = computed(() =>
	authStore.settings.frontendSettings.showSubprojectTasks &&
	projectStore.getChildProjects(projectId.value).length > 0,
)

const groupedTasks = computed(() => {
	const groups = new Map<number, ITask[]>()
	for (const t of tasks.value) {
		if (!groups.has(t.projectId)) groups.set(t.projectId, [])
		groups.get(t.projectId)!.push(t)
	}
	const result: { project: IProject | undefined; tasks: ITask[] }[] = []
	const pushGroup = (pid: number) => {
		const ts = groups.get(pid)
		if (!ts || ts.length === 0) return
		result.push({project: projectStore.projects[pid], tasks: ts})
		groups.delete(pid)
	}
	if (project.value) pushGroup(project.value.id)
	const remaining = [...groups.keys()].sort((a, b) =>
		(projectStore.projects[a]?.title ?? '').localeCompare(projectStore.projects[b]?.title ?? ''))
	remaining.forEach(pushGroup)
	return result
})

const collapsedSections = ref<Set<number>>(new Set())
function toggleSection(pid: number | undefined) {
	if (pid === undefined) return
	const next = new Set(collapsedSections.value)
	if (next.has(pid)) next.delete(pid)
	else next.add(pid)
	collapsedSections.value = next
}
function isSectionCollapsed(pid: number | undefined) {
	return pid !== undefined && collapsedSections.value.has(pid)
}

const isTouchDevice = ref(false)
if (typeof window !== 'undefined') {
	isTouchDevice.value = !window.matchMedia('(hover: hover) and (pointer: fine)').matches
}
const dragHandle = computed(() => isTouchDevice.value ? '.handle' : undefined)

const addTaskRef = ref<typeof AddTask | null>(null)

function focusNewTaskInput() {
	addTaskRef.value?.focusTaskInput()
}

function updateTaskList(task: ITask) {
	if (!isPositionSorting.value) {
		// reload tasks with current filter and sorting
		loadTasks()
	} else {
		allTasks.value = [
			task,
			...allTasks.value,
		]
	}

	baseStore.setHasTasks(true)
}

function updateTasks(updatedTask: ITask) {
	if (projectId.value < 0) {
		// Reload tasks to keep saved filter results in sync
		loadTasks(false)
		return
	}

	for (let t = 0; t < tasks.value.length; t++) {
		if (tasks.value[t].id === updatedTask.id) {
			tasks.value[t] = updatedTask
			break
		}
	}
}

function handleDragStart(e: { item: HTMLElement }) {
	drag.value = true
	const taskId = parseInt(e.item.dataset.taskId ?? '', 10)
	const task = tasks.value.find(t => t.id === taskId)

	if (task) {
		taskStore.setDraggedTask(task)
	}
}

async function saveTaskPosition(e: { originalEvent?: MouseEvent, to: HTMLElement, from: HTMLElement, newIndex: number }) {
	drag.value = false

	// Check if dropped on a sidebar project
	const {moved} = await handleTaskDropToProject(e, (task) => {
		tasks.value = tasks.value.filter(t => t.id !== task.id)
	})

	if (moved) {
		return
	}

	// If dropped outside this list
	if (e.to !== e.from) {
		return
	}

	const task = tasks.value[e.newIndex]
	const taskBefore = tasks.value[e.newIndex - 1] ?? null
	const taskAfter = tasks.value[e.newIndex + 1] ?? null

	const position = calculateItemPosition(taskBefore !== null ? taskBefore.position : null, taskAfter !== null ? taskAfter.position : null)

	await taskPositionService.value.update(new TaskPositionModel({
		position,
		projectViewId: props.viewId,
		taskId: task.id,
	}))
	tasks.value[e.newIndex] = {
		...task,
		position,
	}
}

const taskRefs = ref<(InstanceType<typeof SingleTaskInProject> | null)[]>([])
const focusedIndex = ref(-1)

function setTaskRef(el: InstanceType<typeof SingleTaskInProject> | null, index: number) {
	if (el === null) {
		delete taskRefs.value[index]
	} else {
		taskRefs.value[index] = el
	}
}

function focusTask(index: number) {
	if (index < 0 || index >= tasks.value.length) {
		return
	}

	const taskRef = taskRefs.value[index]

	focusedIndex.value = index
	taskRef?.focus()
}

function handleListNavigation(e: KeyboardEvent) {
	if (e.target instanceof HTMLElement && (e.target.closest('input, textarea, select, [contenteditable="true"]'))) {
		return
	}

	if (e.code === 'KeyJ') {
		e.preventDefault()
		focusTask(Math.min(focusedIndex.value + 1, tasks.value.length - 1))
		return
	}

	if (e.code === 'KeyK') {
		e.preventDefault()
		if (focusedIndex.value === -1) {
			focusTask(tasks.value.length - 1)
			return
		}

		if (focusedIndex.value === 0) {
			addTaskRef.value?.focusTaskInput()
			focusedIndex.value = -1
			return
		}

		focusTask(Math.max(focusedIndex.value - 1, 0))
		return
	}

	if (e.code === 'Enter') {
		if (e.isComposing) {
			return
		}

		// Links and buttons activate natively on Enter; leave them alone
		if (e.target instanceof HTMLElement && e.target.closest('a, button, [role="button"]')) {
			return
		}

		// Only act when a row was focused via J/K roving navigation
		if (focusedIndex.value < 0) {
			return
		}

		e.preventDefault()
		taskRefs.value[focusedIndex.value]?.click(e)
	}
}

onMounted(() => {
	document.addEventListener('keydown', handleListNavigation)
})

onBeforeUnmount(() => {
	document.removeEventListener('keydown', handleListNavigation)
})
</script>

<style lang="scss" scoped>
.filter-container {
	display: flex;
	align-items: center;
	gap: .5rem;

	:deep(.popup) {
		inset-block-start: 3rem;
		inset-inline-end: 0;
		max-inline-size: 300px;
	}
}

.project-section {
	&:not(:first-child) {
		border-block-start: 1px solid var(--border);
	}

	&__header {
		display: flex;
		align-items: center;
		gap: .5rem;
		padding: .75rem 1rem;
		cursor: pointer;
		user-select: none;
		font-weight: bold;

		&:focus-visible {
			outline: 2px solid var(--primary);
			outline-offset: -2px;
		}
	}

	&__chevron {
		font-size: .75rem;
		color: var(--grey-500);
	}

	&__title {
		margin-inline-end: auto;
	}

	&__count {
		color: var(--grey-500);
		font-weight: normal;
	}

	&__tasks {
		padding-block-start: 0;
	}
}

.tasks {
	padding: .5rem;
}

.task-ghost {
	border-radius: $radius;
	background: var(--grey-100);
	border: 2px dashed var(--grey-300);

	* {
		opacity: 0;
	}
}

.list-view__add-task {
	padding: 1rem 1rem 0;
}

.link-share-view .card {
	border: none;
	box-shadow: none;
}

:deep(.single-task .handle) {
	cursor: grab;
	margin-inline-end: .25rem;
	color: var(--grey-400);
}

@media (hover: hover) and (pointer: fine) {
	:deep(.single-task .handle) {
		display: none;
	}
}

:deep(.tasks:not(.dragging-disabled) .single-task) {
	cursor: grab;
	-webkit-touch-callout: none;
	user-select: none;
	touch-action: manipulation;

	&:active {
		cursor: grabbing;
	}
}

.list-view {
	padding-block-end: 1rem;

	:deep(.card) {
		margin-block-end: 0;
	}
}
</style>
