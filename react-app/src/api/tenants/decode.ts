import type { Status, Tenant, WechatConfiguration } from './types'
import { id, invalid, record, string } from '../decode'
function status(value: unknown): Status {
  return value === 'active' || value === 'disabled' ? value : invalid()
}

export function decodeWechatConfiguration(value: unknown): WechatConfiguration {
  const row = record(value)
  return { app_id: string(row.app_id), app_secret: string(row.app_secret) }
}
export function decodeTenant(value: unknown): Tenant {
  const row = record(value)
  return {
    id: id(row.id),
    name: string(row.name),
    hostname: string(row.hostname),
    status: status(row.status),
    created_at: string(row.created_at),
    updated_at: string(row.updated_at),
  }
}
