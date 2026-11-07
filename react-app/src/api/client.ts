import { createRequest } from './request'
import type { ApiOptions } from './request'
import { createAuthApi } from './auth/api'
import { createAccountApi } from './account/api'
import { createMembersApi } from './members'
import { createTenantsApi } from './tenants/api'
import { createKnowledgeApi } from './knowledge/api'
import { createAIApi } from './ai/api'
import { createChatApi } from './chat/api'
import { createStreamRequest } from './stream'
export { ApiError } from './ApiError'

export function createApi(options: ApiOptions) {
  const request = createRequest(options)
  return {
    ...createAuthApi(request),
    ...createAccountApi(request),
    ...createMembersApi(request),
    ...createTenantsApi(request),
    ...createKnowledgeApi(request),
    ...createAIApi(request),
    ...createChatApi(request, createStreamRequest(options)),
  }
}
export type Api = ReturnType<typeof createApi>
