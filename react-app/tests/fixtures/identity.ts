import type { User } from '../../src/api/account/types'
import type { Tenant } from '../../src/api/tenants/types'

export const tenant: Tenant = {
  id: '9007199254740993',
  name: '测试企业',
  hostname: 'acme.test',
  status: 'active',
  registration_enabled: true,
  created_at: '2026-11-05T00:00:00Z',
  updated_at: '2026-11-05T00:00:00Z',
}
export const alice: User = {
  id: '2',
  tenant_id: tenant.id,
  username: 'alice',
  nickname: '小艾',
  avatar_url: '',
  role: 'admin',
  status: 'active',
  created_at: tenant.created_at,
  updated_at: tenant.updated_at,
}
export const bob: User = { ...alice, id: '3', username: 'bob', nickname: '小博', role: 'user' }

export function deferred<T>() {
  let complete: (value: T) => void = () => {
    throw new Error('等待任务尚未初始化')
  }
  const promise = new Promise<T>((resolve) => {
    complete = resolve
  })
  return { promise, resolve: (value: T) => complete(value) }
}
