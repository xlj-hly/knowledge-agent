import type { Request } from '../request'
import { decodeOk } from '../decode'
import type { PasswordChange } from './types'
import { decodeUploadedAvatar, decodeUser } from './decode'

export function createAccountApi(request: Request) {
  return {
    uploadAvatar: (file: File, signal?: AbortSignal) => {
      const body = new FormData()
      body.append('file', file)
      return request('/account/avatar', decodeUploadedAvatar, 'POST', body, signal)
    },
    profile: (body: { nickname: string; avatar_url: string }, signal?: AbortSignal) =>
      request('/account/profile', decodeUser, 'PUT', body, signal),
    password: (body: PasswordChange, signal?: AbortSignal) =>
      request('/account/password', decodeOk, 'POST', body, signal),
    platformPassword: (body: PasswordChange, signal?: AbortSignal) =>
      request('/platform/account/password', decodeOk, 'POST', body, signal),
  }
}
export type AccountApi = ReturnType<typeof createAccountApi>
