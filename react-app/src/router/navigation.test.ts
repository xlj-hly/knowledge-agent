import { expect, test } from 'vitest'
import { loginDestination, routeTitle } from './navigation'

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

test('知识库详情可恢复，配置入口属于企业身份且拒绝路径穿越', () => {
  expect(loginDestination(false, { from: '/knowledge/42?tab=search' })).toBe('/knowledge/42?tab=search')
  expect(loginDestination(false, { from: '/models' })).toBe('/models')
  expect(routeTitle(false, '/knowledge/42')).toBe('知识库详情')
  for (const from of ['/knowledge/0', '/knowledge/42/../settings', '/knowledge/%31', '/knowledge/42/edit']) {
    expect(loginDestination(false, { from })).toBe('/')
  }
  expect(loginDestination(true, { from: '/models' })).toBe('/platform')
})
