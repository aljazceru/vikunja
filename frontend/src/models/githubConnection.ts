import AbstractModel from '@/models/abstractModel'
import type {IGitHubConnection} from '@/modelTypes/IGitHubConnection'
import UserModel from '@/models/user'

export default class GitHubConnectionModel extends AbstractModel<IGitHubConnection> implements IGitHubConnection {
	id = 0
	projectId = 0
	repoOwner = ''
	repoName = ''
	installationId = 0
	token = ''
	enabled = true
	lastSyncedAt: Date = new Date(0)
	lastSyncError = ''
	createdBy = new UserModel()

	created: Date = new Date()
	updated: Date = new Date()

	constructor(data: Partial<IGitHubConnection> = {}) {
		super()
		this.assignData(data)

		if (this.createdBy) {
			this.createdBy = new UserModel(this.createdBy)
		}

		this.lastSyncedAt = new Date(this.lastSyncedAt ?? 0)
		this.created = new Date(this.created ?? 0)
		this.updated = new Date(this.updated ?? 0)
	}

	get repoFullName(): string {
		return `${this.repoOwner}/${this.repoName}`
	}
}
