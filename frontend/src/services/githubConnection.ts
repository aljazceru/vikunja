import AbstractService from '@/services/abstractService'
import GitHubConnectionModel from '@/models/githubConnection'
import type {IGitHubConnection, IGitHubSyncStats} from '@/modelTypes/IGitHubConnection'
import {apiV2Url} from '@/helpers/fetcher'

interface PaginatedConnections {
	items: Partial<IGitHubConnection>[]
	total: number
	page: number
	per_page: number
	total_pages: number
}

// The GitHub integration only exists on /api/v2, hence the absolute URLs. The
// v2 list responses come in a paginated envelope, so getAllForProject unwraps
// it instead of using the generic implementation (which expects a bare array).
export default class GitHubConnectionService extends AbstractService<IGitHubConnection> {
	constructor() {
		super({
			update: apiV2Url('projects/{projectId}/integrations/github/connections/{id}'),
			delete: apiV2Url('projects/{projectId}/integrations/github/connections/{id}'),
		})
	}

	modelFactory(data: Partial<IGitHubConnection>): IGitHubConnection {
		return new GitHubConnectionModel(data)
	}

	async getAllForProject(projectId: number): Promise<IGitHubConnection[]> {
		const cancel = this.setLoading()
		try {
			const {data} = await this.http.get(
				apiV2Url(`projects/${projectId}/integrations/github/connections`),
			)
			const body = data as PaginatedConnections
			return (body.items ?? []).map(entry => this.modelFactory(entry))
		} finally {
			cancel()
		}
	}

	async createConnection(projectId: number, connection: Partial<IGitHubConnection>): Promise<IGitHubConnection> {
		// Only send what the create endpoint consumes — serializing the whole
		// model drags readOnly defaults (created_by, timestamps) into the body,
		// which Huma rejects (e.g. the empty default user's username).
		const {data} = await this.http.post(
			apiV2Url(`projects/${projectId}/integrations/github/connections`),
			{
				repo_owner: connection.repoOwner ?? '',
				repo_name: connection.repoName ?? '',
				installation_id: connection.installationId ?? 0,
				token: connection.token ?? '',
			},
		)
		return this.modelFactory(data)
	}

	async sync(projectId: number, connectionId: number): Promise<IGitHubSyncStats> {
		const {data} = await this.http.post(
			apiV2Url(`projects/${projectId}/integrations/github/connections/${connectionId}/sync`),
		)
		return data as IGitHubSyncStats
	}
}
