import type { Role, User, UserStatus } from './types'
import { id, invalid, record, string } from '../decode'

export function decodeUploadedAvatar(value: unknown): string {
  const path = string(record(value).avatar_url)
  return /^\/api\/avatars\/[1-9]\d*\/[1-9]\d*\/[a-zA-Z0-9_-]+\.(?:png|jpg)$/.test(path) ? path : invalid()
}
function userStatus(value: unknown): UserStatus {
  return value === 'active' || value === 'disabled' ? value : invalid()
}
function role(value: unknown): Role {
  return value === 'admin' || value === 'user' ? value : invalid()
}
export function decodeUser(value: unknown): User {
  const row = record(value)
  return {
    id: id(row.id),
    tenant_id: id(row.tenant_id),
    username: string(row.username),
    nickname: string(row.nickname),
    avatar_url: string(row.avatar_url),
    role: role(row.role),
    status: userStatus(row.status),
    created_at: string(row.created_at),
    updated_at: string(row.updated_at),
  }
}
