import type { PlatformAdmin, TenantSession, PlatformSession, Site } from './types'
import { id, invalid, record, string } from '../decode'
import { decodeTenant } from '../tenants/decode'
import { decodeUser } from '../account/decode'

export function decodeSite(value: unknown): Site {
  const row = record(value)
  const hostname = string(row.hostname)
  if (row.kind === 'tenant') return { kind: 'tenant', hostname, tenant: decodeTenant(row.tenant) }
  if (row.kind === 'platform' || row.kind === 'unknown') return { kind: row.kind, hostname }
  return invalid()
}
export function decodeAdmin(value: unknown): PlatformAdmin {
  const row = record(value)
  return { id: id(row.id), username: string(row.username), created_at: string(row.created_at) }
}

export function decodeIdentity(value: unknown) {
  const row = record(value)
  return { user: decodeUser(row.user), tenant: decodeTenant(row.tenant) }
}
export function decodeLogin(value: unknown): TenantSession {
  const row = record(value)
  return { ...decodeIdentity(row), token: string(row.token), expires_at: string(row.expires_at) }
}
export function decodePlatformLogin(value: unknown): PlatformSession {
  const row = record(value)
  return {
    admin: decodeAdmin(row.admin),
    token: string(row.token),
    expires_at: string(row.expires_at),
  }
}
