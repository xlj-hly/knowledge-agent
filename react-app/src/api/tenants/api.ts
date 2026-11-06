import type { Request } from '../request'
import { decodeOk, decodePage, record } from '../decode'
import type { TenantUpdate, WechatConfigurationUpdate } from './types'
import { decodeUser } from '../account/decode'
import { decodeTenant, decodeWechatConfiguration } from './decode'

export function createTenantsApi(request: Request) {
  const query = (q: string, page: number) =>
    new URLSearchParams({ q, page: String(page), page_size: '20' }).toString()
  return {
    tenantWechat: (tenantId: string, signal?: AbortSignal) =>
      request(`/platform/tenants/${encodeURIComponent(tenantId)}/wechat`, decodeWechatConfiguration, 'GET', undefined, signal),
    saveTenantWechat: (tenantId: string, body: WechatConfigurationUpdate, signal?: AbortSignal) =>
      request(`/platform/tenants/${encodeURIComponent(tenantId)}/wechat`, decodeWechatConfiguration, 'PUT', body, signal),
    updateTenant: (body: { name: string }, signal?: AbortSignal) =>
      request('/admin/tenant', decodeTenant, 'PATCH', body, signal),
    tenants: (q: string, page: number, signal?: AbortSignal) =>
      request(
        `/platform/tenants?${query(q, page)}`,
        decodePage(decodeTenant),
        'GET',
        undefined,
        signal,
      ),
    createTenant: (
      body: { name: string; hostname: string; admin_username: string; admin_password: string },
      signal?: AbortSignal,
    ) =>
      request(
        '/platform/tenants',
        (value) => {
          const row = record(value)
          return { tenant: decodeTenant(row.tenant), user: decodeUser(row.user) }
        },
        'POST',
        body,
        signal,
      ),
    platformUpdateTenant: (tenantId: string, body: TenantUpdate, signal?: AbortSignal) =>
      request(
        `/platform/tenants/${encodeURIComponent(tenantId)}`,
        decodeTenant,
        'PATCH',
        body,
        signal,
      ),
    resetAdminPassword: (
      tenantId: string,
      body: { username: string; new_password: string },
      signal?: AbortSignal,
    ) =>
      request(
        `/platform/tenants/${encodeURIComponent(tenantId)}/reset-admin-password`,
        decodeOk,
        'POST',
        body,
        signal,
      ),
  }
}
export type TenantsApi = ReturnType<typeof createTenantsApi>
