export type Status = 'active' | 'disabled'
export interface Tenant {
  id: string
  name: string
  hostname: string
  status: Status
  created_at: string
  updated_at: string
}

export interface TenantUpdate {
  name: string
  hostname: string
  status: Status
}

export interface WechatConfiguration {
  app_id: string
  app_secret: string
}

export interface WechatConfigurationUpdate {
  app_id: string
  app_secret: string
}
