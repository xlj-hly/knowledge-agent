import { createRequest } from './request'
import type { ApiOptions } from './request'
import { createAuthApi } from './auth/api'
import { createAccountApi } from './account/api'
import { createMembersApi } from './members'
import { createTenantsApi } from './tenants/api'
export { ApiError } from './ApiError'

export function createApi(options: ApiOptions) {
  const request = createRequest(options)
  return {
    ...createAuthApi(request),
    ...createAccountApi(request),
    ...createMembersApi(request),
    ...createTenantsApi(request),
  }
}
export type Api = ReturnType<typeof createApi>
