<script setup lang="ts">
import {computed, onMounted, ref} from 'vue'
import {useI18n} from 'vue-i18n'
import {useTitle} from '@/composables/useTitle'

import XButton from '@/components/input/Button.vue'
import FormField from '@/components/input/FormField.vue'
import Multiselect from '@/components/input/Multiselect.vue'
import Message from '@/components/misc/Message.vue'
import Modal from '@/components/misc/Modal.vue'

import {useAgentService} from '@/services/agent'
import type {IAgent, IAgentCreate} from '@/modelTypes/IAgent'
import type {IProject} from '@/modelTypes/IProject'
import {useProjectStore} from '@/stores/projects'
import {formatDisplayDate} from '@/helpers/time/formatDate'

const {t} = useI18n({useScope: 'global'})
useTitle(() => t('user.settings.agents.title'))

const {getAll: getAgents, createAgent, rotateToken, remove: deleteAgent} = useAgentService()
const projectStore = useProjectStore()

const agents = ref<IAgent[]>([])
const loading = ref(true)
const error = ref<string | null>(null)

// --- create wizard state ---
const showCreateForm = ref(false)
const newName = ref('')
const newPreset = ref<IAgentCreate['preset']>('read-write')
const newProjects = ref<IProject[]>([])
const newProjectPermission = ref(1)
const newToken = ref<string | null>(null)
const createError = ref<string | null>(null)
const creating = ref(false)

const PRESETS = computed<{value: IAgentCreate['preset'], title: string, description: string}[]>(() => [
	{value: 'read-only', title: t('user.settings.agents.presets.readOnly'), description: t('user.settings.agents.presets.readOnlyDescription')},
	{value: 'comment-only', title: t('user.settings.agents.presets.commentOnly'), description: t('user.settings.agents.presets.commentOnlyDescription')},
	{value: 'read-write', title: t('user.settings.agents.presets.readWrite'), description: t('user.settings.agents.presets.readWriteDescription')},
])

const projects = computed(() => Object.values(projectStore.projects ?? {}))

const PERMISSIONS = [
	{value: 0, title: t('project.share.permission.read')},
	{value: 1, title: t('project.share.permission.readWrite')},
	{value: 2, title: t('project.share.permission.admin')},
]

async function loadAgents() {
	loading.value = true
	error.value = null
	try {
		agents.value = await getAgents()
	} catch (e: unknown) {
		const err = e as {response?: {data?: {message?: string}}}
		error.value = err?.response?.data?.message ?? String(e)
	} finally {
		loading.value = false
	}
}

function resetCreateForm() {
	newName.value = ''
	newPreset.value = 'read-write'
	newProjects.value = []
	newProjectPermission.value = 1
	newToken.value = null
	createError.value = null
}

async function create() {
	createError.value = null
	if (!newName.value.trim()) {
		createError.value = t('user.settings.agents.nameRequired')
		return
	}
	creating.value = true
	try {
		const {agent, token} = await createAgent({
			name: newName.value.trim(),
			preset: newPreset.value,
			projects: newProjects.value.map(project => ({
				projectId: project.id,
				permission: newProjectPermission.value,
			})),
		})
		agents.value.push(agent)
		newToken.value = token
	} catch (e: unknown) {
		const err = e as {response?: {data?: {message?: string}}}
		createError.value = err?.response?.data?.message ?? String(e)
	} finally {
		creating.value = false
	}
}

const rotatedTokens = ref<Record<number, string>>({})

async function rotate(agent: IAgent) {
	try {
		const {agent: rotated, token} = await rotateToken(agent.id)
		const idx = agents.value.findIndex(a => a.id === agent.id)
		if (idx >= 0) {
			agents.value[idx] = rotated
		}
		rotatedTokens.value[agent.id] = token
	} catch (e: unknown) {
		const err = e as {response?: {data?: {message?: string}}}
		error.value = err?.response?.data?.message ?? String(e)
	}
}

const showDeleteModal = ref(false)
const agentToDelete = ref<IAgent>()

async function remove() {
	const agent = agentToDelete.value
	if (!agent) return
	try {
		await deleteAgent(agent.id)
		agents.value = agents.value.filter(a => a.id !== agent.id)
		showDeleteModal.value = false
		agentToDelete.value = undefined
	} catch (e: unknown) {
		const err = e as {response?: {data?: {message?: string}}}
		error.value = err?.response?.data?.message ?? String(e)
	}
}

function presetTitle(agent: IAgent): string {
	const perms = agent.tokens?.[0]?.permissions ?? {}
	const taskPerms = perms.tasks ?? []
	const commentPerms = perms.tasksComments ?? []
	if (taskPerms.includes('create') || taskPerms.includes('update')) return t('user.settings.agents.presets.readWrite')
	if (commentPerms.includes('create')) return t('user.settings.agents.presets.commentOnly')
	return t('user.settings.agents.presets.readOnly')
}

function heartbeatLabel(agent: IAgent): string {
	if (!agent.lastUsedAt) return t('user.settings.agents.neverUsed')
	return t('user.settings.agents.lastUsed', {date: formatDisplayDate(agent.lastUsedAt)})
}

onMounted(async () => {
	await Promise.all([loadAgents(), projectStore.loadAllProjects()])
})
</script>

<template>
	<div class="content">
		<h2>{{ $t('user.settings.agents.title') }}</h2>
		<p>{{ $t('user.settings.agents.description') }}</p>

		<Message
			v-if="error"
			variant="danger"
		>
			{{ error }}
		</Message>

		<div
			v-if="newToken"
			class="card agent-created"
		>
			<h3>{{ $t('user.settings.agents.created.title') }}</h3>
			<Message variant="warning">
				{{ $t('user.settings.agents.created.tokenOnce') }}<br>
				<code class="agent-token">{{ newToken }}</code>
			</Message>
			<p class="agent-mcp-hint">
				{{ $t('user.settings.agents.created.mcpHint') }} <code>{{ $t('user.settings.agents.created.mcpEndpoint') }}</code>
			</p>
			<XButton
				variant="secondary"
				@click="newToken = null"
			>
				{{ $t('misc.done') }}
			</XButton>
		</div>

		<div
			v-else-if="showCreateForm"
			class="card agent-form"
		>
			<FormField :label="$t('user.settings.agents.nameLabel')">
				<input
					v-model="newName"
					class="input"
					:placeholder="$t('user.settings.agents.namePlaceholder')"
				>
			</FormField>

			<FormField :label="$t('user.settings.agents.presetLabel')">
				<div
					v-for="preset in PRESETS"
					:key="preset.value"
					class="preset-option"
					:class="{'is-active': newPreset === preset.value}"
					role="button"
					@click="newPreset = preset.value"
				>
					<strong>{{ preset.title }}</strong>
					<span>{{ preset.description }}</span>
				</div>
			</FormField>

			<FormField :label="$t('user.settings.agents.projectsLabel')">
				<Multiselect
					v-model="newProjects"
					:search-results="projects"
					:multiple="true"
					:show-empty="true"
					label="title"
					track-by="id"
					:placeholder="$t('user.settings.agents.projectsPlaceholder')"
				/>
			</FormField>

			<FormField
				v-if="newProjects.length > 0"
				:label="$t('user.settings.agents.permissionLabel')"
			>
				<div class="control">
					<label
						v-for="perm in PERMISSIONS"
						:key="perm.value"
						class="radio"
					>
						<input
							v-model="newProjectPermission"
							type="radio"
							:value="perm.value"
						>
						{{ perm.title }}
					</label>
				</div>
			</FormField>

			<Message
				v-if="createError"
				variant="danger"
			>
				{{ createError }}
			</Message>

			<div class="actions">
				<XButton
					:loading="creating"
					@click="create"
				>
					{{ $t('user.settings.agents.create') }}
				</XButton>
				<XButton
					variant="tertiary"
					@click="resetCreateForm(); showCreateForm = false"
				>
					{{ $t('misc.cancel') }}
				</XButton>
			</div>
		</div>

		<XButton
			v-else
			icon="plus"
			class="mbe-4"
			@click="showCreateForm = true"
		>
			{{ $t('user.settings.agents.create') }}
		</XButton>

		<Message
			v-if="!loading && agents.length === 0 && !newToken && !showCreateForm"
		>
			{{ $t('user.settings.agents.noAgents') }}
		</Message>

		<div
			v-for="agent in agents"
			:key="agent.id"
			class="card agent-card"
		>
			<div class="agent-header">
				<strong>{{ agent.username }}</strong>
				<span v-if="agent.name"> — {{ agent.name }}</span>
				<span class="agent-preset">{{ presetTitle(agent) }}</span>
				<span class="agent-heartbeat">{{ heartbeatLabel(agent) }}</span>
			</div>

			<div
				v-if="agent.projects?.length"
				class="agent-projects"
			>
				<span
					v-for="project in agent.projects"
					:key="project.projectId"
					class="tag"
				>
					{{ project.title }}
				</span>
			</div>

			<Message
				v-if="rotatedTokens[agent.id]"
				variant="warning"
			>
				{{ $t('user.settings.agents.created.tokenOnce') }}<br>
				<code class="agent-token">{{ rotatedTokens[agent.id] }}</code>
			</Message>

			<div class="actions">
				<XButton
					variant="secondary"
					@click="rotate(agent)"
				>
					{{ $t('user.settings.agents.rotateToken') }}
				</XButton>
				<XButton
					variant="tertiary"
					class="is-danger"
					@click="() => {agentToDelete = agent; showDeleteModal = true}"
				>
					{{ $t('misc.delete') }}
				</XButton>
			</div>
		</div>

		<Modal
			:enabled="showDeleteModal"
			@close="showDeleteModal = false"
			@submit="remove()"
		>
			<template #header>
				{{ $t('user.settings.agents.delete.header') }}
			</template>

			<template #text>
				<p>{{ $t('user.settings.agents.delete.text1', {username: agentToDelete?.username}) }}</p>
				<p>{{ $t('user.settings.agents.delete.text2') }}</p>
			</template>
		</Modal>
	</div>
</template>

<style lang="scss" scoped>
.agent-form {
	display: flex;
	flex-direction: column;
	gap: .75rem;
}

.preset-option {
	display: flex;
	flex-direction: column;
	gap: .25rem;
	padding: .5rem .75rem;
	border: 1px solid var(--grey-200);
	border-radius: 4px;
	cursor: pointer;
	font-size: .9rem;

	&.is-active {
		border-color: var(--primary);
		background-color: var(--primary-light);
	}
}

.agent-card {
	padding: 1rem;
	margin-block-start: 1rem;
}

.agent-header {
	display: flex;
	gap: .5rem;
	align-items: center;
	margin-block-end: .5rem;
	flex-wrap: wrap;
}

.agent-preset {
	font-size: .8rem;
	padding: .1rem .5rem;
	border-radius: 3px;
	background: var(--grey-200);
}

.agent-heartbeat {
	margin-inline-start: auto;
	font-size: .85rem;
	color: var(--grey-500);
}

.agent-projects {
	display: flex;
	gap: .25rem;
	flex-wrap: wrap;
	margin-block-end: .75rem;
}

.agent-token {
	user-select: all;
	word-break: break-all;
}

.agent-mcp-hint code {
	user-select: all;
}

.agent-created {
	padding: 1rem;
	margin-block-end: 1rem;
}

.actions {
	display: flex;
	gap: .5rem;
}
</style>
