import type {IAbstract} from '@/modelTypes/IAbstract'
import type {IApiToken} from '@/modelTypes/IApiToken'

export interface IAgentProject {
	projectId: number
	permission: number // 0 = read, 1 = read/write, 2 = admin
	title: string
}

export interface IAgent extends IAbstract {
	id: number
	username: string
	name: string
	status: number
	projects: IAgentProject[]
	tokens: IApiToken[]
	lastUsedAt: Date | null
	created: Date | null
}

export interface IAgentCreate {
	name: string
	preset: 'read-only' | 'comment-only' | 'read-write'
	projects: {projectId: number, permission: number}[]
	username?: string
	expiresAt?: Date | null
}
