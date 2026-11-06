import type { Request } from '../request'
import { decodeOk, record } from '../decode'
import type { Credentials } from '../account/types'
import { decodeAdmin, decodeIdentity, decodeLogin, decodePlatformLogin, decodeSite } from './decode'

export function createAuthApi(request: Request) {
  return {
    site: (signal?: AbortSignal) => request('/site', decodeSite, 'GET', undefined, signal),
    login: (body: Credentials, signal?: AbortSignal) =>
      request('/auth/login', decodeLogin, 'POST', body, signal),
    register: (body: Credentials & { nickname: string }, signal?: AbortSignal) =>
      request('/auth/register', decodeLogin, 'POST', body, signal),
    me: (signal?: AbortSignal) => request('/auth/me', decodeIdentity, 'GET', undefined, signal),
    logout: (signal?: AbortSignal) => request('/auth/logout', decodeOk, 'POST', undefined, signal),
    platformLogin: (body: Credentials, signal?: AbortSignal) =>
      request('/platform/auth/login', decodePlatformLogin, 'POST', body, signal),
    platformMe: (signal?: AbortSignal) =>
      request(
        '/platform/auth/me',
        (value) => ({ admin: decodeAdmin(record(value).admin) }),
        'GET',
        undefined,
        signal,
      ),
    platformLogout: (signal?: AbortSignal) =>
      request('/platform/auth/logout', decodeOk, 'POST', undefined, signal),
  }
}
export type AuthApi = ReturnType<typeof createAuthApi>
