import { expect, test } from 'vitest'
import { loginDestination } from './navigation'

test('登录恢复只接受本身份空间的已定义站内路由，拒绝外站、穿越路径和登录循环', () => {
  expect(loginDestination(false, { from: '/account?tab=profile#nickname' })).toBe('/account?tab=profile#nickname')
  expect(loginDestination(true, { from: '/platform/tenants' })).toBe('/platform/tenants')
  for (const from of ['https://other.test/account', '//other.test/account', '/platform/tenants', '/login', '/missing', '/settings/../platform']) {
    expect(loginDestination(false, { from })).toBe('/')
  }
  for (const from of ['/account', '/platform/login', '/platform/../account', '/platform/missing']) {
    expect(loginDestination(true, { from })).toBe('/platform')
  }
})
