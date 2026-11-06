import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { createMemoryRouter, RouterProvider } from 'react-router'
import App from '../../src/App'
import { alice, bob, tenant } from '../fixtures/identity'

afterEach(() => { cleanup(); vi.unstubAllGlobals(); sessionStorage.clear() })

function boot(path: string, user = alice, authenticated = true, kind: 'platform' | 'tenant' | 'unknown' = 'tenant') {
  if (authenticated) sessionStorage.setItem(`yunxu.tenant.session.${window.location.hostname}`, JSON.stringify({ state: { token: 'tenant-session' }, version: 0 }))
  const calls: string[] = []
  vi.stubGlobal('fetch', async (url: string) => {
    calls.push(url)
    const data = url.endsWith('/site')
      ? { kind, hostname: window.location.hostname, ...(kind === 'tenant' ? { tenant } : {}) }
      : url.endsWith('/auth/login')
      ? { token: 'new-token', expires_at: '2099-01-01T00:00:00Z', user, tenant }
      : url.endsWith('/admin/users') ? { items: [user], total: 1, page: 1, page_size: 20 }
      : { user, tenant }
    return new Response(JSON.stringify({ data }))
  })
  const router = createMemoryRouter([{ path: '*', element: <App /> }], { initialEntries: [path] })
  render(<RouterProvider router={router} />)
  return { router, calls }
}

test('直接访问资料地址时验证会话并加载资料，导航与后退前进同步 URL 和激活菜单', async () => {
  const { router } = boot('/account')
  expect(await screen.findByLabelText('昵称')).toHaveProperty('value', alice.nickname)
  expect(screen.getByRole('link', { name: '个人账号' }).getAttribute('aria-current')).toBe('page')
  fireEvent.click(screen.getByRole('link', { name: '工作台' }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/'))
  expect(await screen.findByRole('heading', { name: `你好，${alice.nickname}` })).toBeTruthy()
  await act(() => router.navigate(-1))
  expect(await screen.findByLabelText('昵称')).toHaveProperty('value', alice.nickname)
  expect(router.state.location.pathname).toBe('/account')
  await act(() => router.navigate(1))
  expect(await screen.findByRole('heading', { name: `你好，${alice.nickname}` })).toBeTruthy()
  expect(router.state.location.pathname).toBe('/')
})

test('未登录直达受保护页面先进入登录，成功后恢复原地址', async () => {
  const { router } = boot('/settings', alice, false)
  fireEvent.change(await screen.findByLabelText('用户名'), { target: { value: 'alice' } })
  expect(router.state.location.pathname).toBe('/login')
  fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'valid-password' } })
  fireEvent.click(screen.getByRole('button', { name: '登录工作空间' }))
  expect(await screen.findByRole('heading', { name: '企业设置' })).toBeTruthy()
  expect(router.state.location.pathname).toBe('/settings')
})

test('普通成员直接访问管理员页面显示无权访问，未发出管理员请求', async () => {
  const { calls, router } = boot('/members', bob)
  expect(await screen.findByRole('heading', { name: '无权访问' })).toBeTruthy()
  expect(calls.some(url => url.includes('/admin/'))).toBe(false)
  expect(screen.queryByRole('link', { name: '成员管理' })).toBeNull()
  fireEvent.click(screen.getByRole('link', { name: '返回工作台' }))
  expect(await screen.findByRole('heading', { name: `你好，${bob.nickname}` })).toBeTruthy()
  expect(router.state.location.pathname).toBe('/')
})

test('未知地址展示 404 并可返回当前空间，企业域名不开放平台入口', async () => {
  const { router } = boot('/not-a-page')
  expect(await screen.findByRole('heading', { name: '页面不存在' })).toBeTruthy()
  fireEvent.click(screen.getByRole('link', { name: '返回工作台' }))
  expect(await screen.findByRole('heading', { name: `你好，${alice.nickname}` })).toBeTruthy()
  await act(() => router.navigate('/platform/tenants'))
  expect(await screen.findByRole('heading', { name: '无权访问' })).toBeTruthy()
  expect(screen.queryByRole('heading', { name: '欢迎回到管理控制台' })).toBeNull()
})

test('平台域名根地址进入平台登录，忽略企业会话', async () => {
  const { router, calls } = boot('/', alice, true, 'platform')
  expect(await screen.findByRole('heading', { name: '欢迎回到管理控制台' })).toBeTruthy()
  expect(router.state.location.pathname).toBe('/platform/login')
  expect(calls.some(url => url.endsWith('/auth/me'))).toBe(false)
})

test('未绑定域名没有任何身份登录表单', async () => {
  const { calls } = boot('/platform/login', alice, false, 'unknown')
  expect(await screen.findByRole('heading', { name: '域名未绑定' })).toBeTruthy()
  expect(screen.queryByLabelText('用户名')).toBeNull()
  expect(calls).toEqual(['/api/site'])
})

test('站点发现失败可重试，成功后才呈现企业登录', async () => {
  let attempts = 0
  vi.stubGlobal('fetch', async () => {
    attempts += 1
    if (attempts === 1) throw new Error('offline')
    return new Response(JSON.stringify({ data: { kind: 'tenant', hostname: tenant.hostname, tenant } }))
  })
  render(<RouterProvider router={createMemoryRouter([{ path: '*', element: <App /> }])} />)
  expect((await screen.findByRole('alert')).textContent).toContain('无法连接服务')
  expect(screen.queryByLabelText('用户名')).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: '重新加载' }))
  expect(await screen.findByLabelText('用户名')).toBeTruthy()
})

test('已绑定但停用的企业显示停用原因，不提供登录表单', async () => {
  vi.stubGlobal('fetch', async () => new Response(JSON.stringify({ data: {
    kind: 'tenant', hostname: tenant.hostname, tenant: { ...tenant, status: 'disabled' },
  } })))
  render(<RouterProvider router={createMemoryRouter([{ path: '*', element: <App /> }])} />)
  expect((await screen.findByRole('alert')).textContent).toContain('停用')
  expect(screen.queryByLabelText('用户名')).toBeNull()
  expect(screen.queryByRole('heading', { name: '域名未绑定' })).toBeNull()
})

test('登录持久化失败仍进入工作台，并向用户显示无法保存会话的原因', async () => {
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new Error('storage denied')
  })
  boot('/', alice, false)
  fireEvent.change(await screen.findByLabelText('用户名'), { target: { value: 'alice' } })
  fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'valid-password' } })
  fireEvent.click(screen.getByRole('button', { name: '登录工作空间' }))
  expect(await screen.findByRole('heading', { name: `你好，${alice.nickname}` })).toBeTruthy()
  expect((await screen.findByRole('alert')).textContent).toContain('保存会话')
})

test.each([
  '{bad-json',
  JSON.stringify({ state: { token: 'bad\u0000token' }, version: 0 }),
])('无效本地会话显示重新登录原因，不携带坏令牌请求身份接口：%s', async (stored) => {
  sessionStorage.setItem(`yunxu.tenant.session.${window.location.hostname}`, stored)
  const { calls } = boot('/', alice, false)
  expect(await screen.findByRole('button', { name: '登录工作空间' })).toBeTruthy()
  expect((await screen.findByRole('alert')).textContent).toContain('重新登录')
  expect(calls.some(url => url.endsWith('/auth/me'))).toBe(false)
})
