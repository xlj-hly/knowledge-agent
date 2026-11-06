import type { User } from '../account/types'
import type { Tenant } from '../tenants/types'
export interface PlatformAdmin {
  id: string
  username: string
  created_at: string
}

export type Site =
  | { kind: 'platform'; hostname: string }
  | { kind: 'unknown'; hostname: string }
  | { kind: 'tenant'; hostname: string; tenant: Tenant }

export interface TenantSession {
  token: string
  expires_at: string
  user: User
  tenant: Tenant
}
export interface PlatformSession {
  token: string
  expires_at: string
  admin: PlatformAdmin
}
