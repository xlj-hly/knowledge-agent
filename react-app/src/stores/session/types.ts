import type { PlatformAdmin } from '../../api/auth/types'
import type { User } from '../../api/account/types'
import type { Tenant } from '../../api/tenants/types'

export type Identity = { admin: PlatformAdmin } | { user: User; tenant: Tenant }

export interface SessionState {
  token: string
  version: number
  notice: string
  storageNotice: string
  changeToken: (token: string) => void
  setNotice: (notice: string) => void
  expire: (version: number) => void
}
