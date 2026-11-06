import type { Request } from './request'
import { decodeOk, decodePage } from './decode'
import type { Credentials, Role, UserStatus } from './account/types'
import { decodeUser } from './account/decode'

export function createMembersApi(request: Request) {
  const query = (q: string, page: number) =>
    new URLSearchParams({ q, page: String(page), page_size: '20' }).toString()
  return {
    users: (q: string, page: number, signal?: AbortSignal) =>
      request(`/admin/users?${query(q, page)}`, decodePage(decodeUser), 'GET', undefined, signal),
    createUser: (body: Credentials & { nickname: string; role: Role }, signal?: AbortSignal) =>
      request('/admin/users', decodeUser, 'POST', body, signal),
    updateUser: (userId: string, body: { role: Role; status: UserStatus }, signal?: AbortSignal) =>
      request(`/admin/users/${encodeURIComponent(userId)}`, decodeUser, 'PATCH', body, signal),
    resetUserPassword: (userId: string, new_password: string, signal?: AbortSignal) =>
      request(
        `/admin/users/${encodeURIComponent(userId)}/reset-password`,
        decodeOk,
        'POST',
        { new_password },
        signal,
      ),
  }
}
export type MembersApi = ReturnType<typeof createMembersApi>
