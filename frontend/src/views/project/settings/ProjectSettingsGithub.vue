<script lang="ts" setup>
import {computed, ref, watchEffect} from 'vue'
import {useRoute} from 'vue-router'
import {useI18n} from 'vue-i18n'
import {useTitle} from '@vueuse/core'

import ProjectService from '@/services/project'
import ProjectModel from '@/models/project'
import type {IProject} from '@/modelTypes/IProject'
import type {IGitHubConnection} from '@/modelTypes/IGitHubConnection'
import GitHubConnectionModel from '@/models/githubConnection'
import GitHubConnectionService from '@/services/githubConnection'

import CreateEdit from '@/components/misc/CreateEdit.vue'
import XButton from '@/components/input/Button.vue'
import FormField from '@/components/input/FormField.vue'
import Message from '@/components/misc/Message.vue'

import {useBaseStore} from '@/stores/base'
import {success} from '@/message'
import {formatDisplayDate} from '@/helpers/time/formatDate'

defineOptions({name: 'ProjectSettingGithub'})

const {t} = useI18n({useScope: 'global'})

const project = ref<IProject>()
useTitle(() => t('project.github.title'))

const route = useRoute()
const projectId = computed(() => route.params.projectId !== undefined
	? parseInt(route.params.projectId as string)
	: undefined,
)

const connections = ref<IGitHubConnection[]>([])
const service = new GitHubConnectionService()
const loading = ref(false)

const newRepo = ref('')
const newToken = ref('')
const newInstallationId = ref('')
const authMode = ref<'token' | 'app'>('token')
const createError = ref<string | null>(null)
const creating = ref(false)
const syncing = ref<Record<number, boolean>>({})

async function loadProject(loadProjectId: number) {
	const projectService = new ProjectService()
	const newProject = await projectService.get(new ProjectModel({id: loadProjectId}))
	await useBaseStore().handleSetCurrentProject({project: newProject})
	project.value = newProject
	await loadConnections()
}

async function loadConnections() {
	if (!project.value) {
		return
	}
	loading.value = true
	try {
		connections.value = await service.getAllForProject(project.value.id)
	} finally {
		loading.value = false
	}
}

watchEffect(() => projectId.value !== undefined && loadProject(projectId.value))

async function createConnection() {
	if (!project.value) {
		return
	}
	createError.value = null
	creating.value = true
	const [repoOwner, ...rest] = newRepo.value.trim().replace(/^https:\/\/github\.com\//, '').replace(/\/$/, '').split('/')
	const repoName = rest.join('/')
	if (!repoOwner || !repoName) {
		createError.value = t('project.github.create.invalidRepo')
		creating.value = false
		return
	}

	try {
		await service.createConnection(project.value.id, new GitHubConnectionModel({
			repoOwner,
			repoName,
			installationId: authMode.value === 'app' ? Number(newInstallationId.value) : 0,
			token: authMode.value === 'app' ? '' : newToken.value,
		} as Partial<IGitHubConnection>))
		newRepo.value = ''
		newToken.value = ''
		newInstallationId.value = ''
		success({message: t('project.github.create.success')})
		await loadConnections()
	} catch (e: unknown) {
		const err = e as {response?: {data?: {detail?: string, message?: string}}}
		createError.value = err?.response?.data?.detail ?? err?.response?.data?.message ?? String(e)
	} finally {
		creating.value = false
	}
}

async function toggleEnabled(connection: IGitHubConnection) {
	const updated = await service.update(new GitHubConnectionModel({
		...connection,
		enabled: !connection.enabled,
	})) as IGitHubConnection
	const idx = connections.value.findIndex(c => c.id === connection.id)
	if (idx >= 0) {
		connections.value[idx] = updated
	}
}

async function syncNow(connection: IGitHubConnection) {
	if (!project.value) {
		return
	}
	syncing.value[connection.id] = true
	try {
		const stats = await service.sync(project.value.id, connection.id)
		success({message: t('project.github.sync.success', {
			created: stats.created,
			updated: stats.updated,
			unchanged: stats.unchanged,
		})})
		await loadConnections()
	} finally {
		syncing.value[connection.id] = false
	}
}

async function removeConnection(connection: IGitHubConnection) {
	if (!project.value) {
		return
	}
	await service.delete(new GitHubConnectionModel({
		id: connection.id,
		projectId: project.value.id,
	}))
	success({message: t('project.github.deleteSuccess')})
	await loadConnections()
}

function formatSyncedAt(connection: IGitHubConnection): string {
	if (!connection.lastSyncedAt || connection.lastSyncedAt.getFullYear() <= 1) {
		return t('project.github.neverSynced')
	}
	return formatDisplayDate(connection.lastSyncedAt)
}
</script>

<template>
	<CreateEdit
		:title="$t('project.github.title')"
		:has-primary-action="false"
		:wide="true"
	>
		<p>{{ $t('project.github.description') }}</p>

		<form
			class="github-add-form"
			@submit.prevent="createConnection"
		>
			<FormField
				:label="$t('project.github.create.repoLabel')"
				:error="createError"
			>
				<input
					v-model="newRepo"
					v-focus
					class="input"
					placeholder="owner/repo"
				>
			</FormField>
			<FormField
				v-if="authMode === 'token'"
				:label="$t('project.github.create.tokenLabel')"
			>
				<input
					v-model="newToken"
					class="input"
					type="password"
					:placeholder="$t('project.github.create.tokenPlaceholder')"
				>
			</FormField>
			<FormField
				v-else
				:label="$t('project.github.create.installationLabel')"
			>
				<input
					v-model="newInstallationId"
					class="input"
					type="number"
					placeholder="12345678"
				>
			</FormField>
			<div class="github-auth-mode">
				<label class="radio">
					<input
						v-model="authMode"
						type="radio"
						value="token"
					>
					{{ $t('project.github.create.modeToken') }}
				</label>
				<label class="radio">
					<input
						v-model="authMode"
						type="radio"
						value="app"
					>
					{{ $t('project.github.create.modeApp') }}
				</label>
			</div>
			<XButton
				type="submit"
				:loading="creating"
				icon="code-branch"
			>
				{{ $t('project.github.create.submit') }}
			</XButton>
		</form>

		<Message
			v-if="loading"
		>
			{{ $t('loading') }}
		</Message>

		<div
			v-for="connection in connections"
			:key="connection.id"
			class="github-connection card mb-2"
		>
			<div class="github-connection-header">
				<strong>{{ connection.repoOwner }}/{{ connection.repoName }}</strong>
				<span
					class="tag"
					:class="connection.enabled ? 'is-success' : 'is-light'"
				>
					{{ connection.enabled ? $t('project.github.state.active') : $t('project.github.state.paused') }}
				</span>
			</div>
			<p class="is-size-7 has-text-grey">
				{{ $t('project.github.lastSynced') }}: {{ formatSyncedAt(connection) }}
			</p>
			<p
				v-if="connection.lastSyncError"
				class="has-text-danger"
			>
				{{ connection.lastSyncError }}
			</p>
			<div class="github-connection-actions buttons">
				<XButton
					variant="secondary"
					icon="history"
					:loading="syncing[connection.id]"
					:disabled="!connection.enabled"
					@click="syncNow(connection)"
				>
					{{ $t('project.github.syncNow') }}
				</XButton>
				<XButton
					variant="secondary"
					@click="toggleEnabled(connection)"
				>
					{{ connection.enabled ? $t('project.github.pause') : $t('project.github.resume') }}
				</XButton>
				<XButton
					variant="secondary"
					danger
					@click="removeConnection(connection)"
				>
					{{ $t('misc.delete') }}
				</XButton>
			</div>
		</div>

		<p
			v-if="!loading && connections.length === 0"
			class="has-text-grey"
		>
			{{ $t('project.github.noConnections') }}
		</p>
	</CreateEdit>
</template>

<style scoped lang="scss">
.github-add-form {
	margin-block-end: 1rem;
}

.github-auth-mode {
	display: flex;
	gap: .75rem;
	margin-block-end: .5rem;
}

.github-connection-header {
	display: flex;
	align-items: center;
	justify-content: space-between;
	gap: .5rem;
	margin-block-end: .25rem;
}

.github-connection-actions {
	margin-block-start: .5rem;
}
</style>
