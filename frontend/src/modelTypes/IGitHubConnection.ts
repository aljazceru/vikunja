import type {IAbstract} from './IAbstract'
import type {IUser} from '@/modelTypes/IUser'

export interface IGitHubConnection extends IAbstract {
	id: number
	projectId: number
	repoOwner: string
	repoName: string
	installationId: number
	token: string
	enabled: boolean
	lastSyncedAt: Date
	lastSyncError: string
	createdBy: IUser

	created: Date
	updated: Date
}

export interface IGitHubSyncStats {
	created: number
	updated: number
	unchanged: number
}
