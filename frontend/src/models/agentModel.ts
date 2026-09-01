import AbstractModel from '@/models/abstractModel'
import type {IAgent, IAgentProject} from '@/modelTypes/IAgent'
import type {IApiToken} from '@/modelTypes/IApiToken'

export default class AgentModel extends AbstractModel<IAgent> {
	id = 0
	username = ''
	name = ''
	status = 0
	projects: IAgentProject[] = []
	tokens: IApiToken[] = []
	lastUsedAt: Date | null = null
	created: Date = new Date(0)

	constructor(data: Partial<IAgent> = {}) {
		super()

		this.assignData(data)

		this.created = this.created ? new Date(this.created) : new Date(0)
		this.lastUsedAt = this.lastUsedAt ? new Date(this.lastUsedAt) : null
	}
}
