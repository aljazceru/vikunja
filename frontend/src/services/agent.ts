import {AuthenticatedHTTPFactory, apiV2Url} from '@/helpers/fetcher'
import {objectToCamelCase, objectToSnakeCase} from '@/helpers/case'

import type {IAgent, IAgentCreate} from '@/modelTypes/IAgent'

export function parseAgent(raw: Record<string, unknown>): IAgent {
	const a = objectToCamelCase(raw) as Record<string, unknown>
	return {
		id: a.id as number,
		username: a.username as string,
		name: a.name as string,
		status: a.status as number,
		projects: (a.projects ?? []) as IAgent['projects'],
		tokens: (a.tokens ?? []) as IAgent['tokens'],
		lastUsedAt: a.lastUsedAt ? new Date(a.lastUsedAt as string) : null,
		created: a.created ? new Date(a.created as string) : null,
		maxPermission: null,
	}
}

export interface AgentCreateResult {
	agent: IAgent
	token: string
}

export function useAgentService() {
	const http = AuthenticatedHTTPFactory()

	async function getAll(): Promise<IAgent[]> {
		const {data} = await http.get(apiV2Url('agents'))
		return (data.items ?? []).map(parseAgent)
	}

	async function get(id: number): Promise<IAgent> {
		const {data} = await http.get(apiV2Url(`agents/${id}`))
		return parseAgent(data)
	}

	async function createAgent(payload: IAgentCreate): Promise<AgentCreateResult> {
		const {data} = await http.post(apiV2Url('agents'), objectToSnakeCase({
			name: payload.name,
			preset: payload.preset,
			projects: (payload.projects ?? []).map(p => ({
				project_id: p.projectId,
				permission: p.permission,
			})),
			username: payload.username || undefined,
			expires_at: payload.expiresAt ? new Date(payload.expiresAt).toISOString() : undefined,
		}))
		return {agent: parseAgent(data.agent), token: data.token}
	}

	async function rotateToken(agentId: number, preset?: IAgentCreate['preset']): Promise<AgentCreateResult> {
		const {data} = await http.post(apiV2Url(`agents/${agentId}/rotate-token`), {
			preset: preset || undefined,
		})
		return {agent: parseAgent(data.agent), token: data.token}
	}

	async function remove(agentId: number): Promise<void> {
		await http.delete(apiV2Url(`agents/${agentId}`))
	}

	return {getAll, get, createAgent, rotateToken, remove}
}
