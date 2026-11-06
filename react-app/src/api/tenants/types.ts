export type Status = 'active' | 'disabled'
export interface Tenant {
  id: string
  name: string
  hostname: string
  status: Status
  registration_enabled: boolean
  created_at: string
  updated_at: string
}

export interface TenantUpdate {
  name: string
  hostname: string
  status: Status
  registration_enabled: boolean
}

export interface WechatConfiguration {
  app_id: string
  has_secret: boolean
}

export interface WechatConfigurationUpdate {
  app_id: string
  app_secret?: string
}
