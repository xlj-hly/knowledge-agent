import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { StrictMode } from 'react'
import { useSession } from './useSession'
import { ApiError } from '../../api/ApiError'
import { deferred } from '../../../tests/fixtures/identity'

const options = { storageKey: 'test.tenant.session' }
afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  sessionStorage.clear()
})

function unauthorized() {
  return new Response(JSON.stringify({ error: { code: 'unauthorized', message: '会话已失效' } }), {
    status: 401,
  })
}

test('会话存储不可读写时仍能登录，401 仍清理内存会话', async () => {
  vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
    throw new Error('storage denied')
  })
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new Error('storage denied')
  })
  vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => {
    throw new Error('storage denied')
  })
  vi.stubGlobal('fetch', async () => unauthorized())
  const { result } = renderHook(() => useSession(options))
  act(() => result.current.changeToken('memory-session'))
  expect(result.current.token).toBe('memory-session')
  expect(result.current.storageNotice).toContain('浏览器未允许保存会话')
  await act(async () => {
    await expect(result.current.api.me()).rejects.toBeInstanceOf(ApiError)
  })
  expect(result.current.token).toBe('')
  expect(result.current.notice).toBe('登录状态已失效，请重新登录。')
  expect(result.current.storageNotice).toContain('清理会话')
})

test('即使存储保留旧令牌，旧请求 401 也不能清理新登录或污染提示', async () => {
  const stored = JSON.stringify({ state: { token: 'old-session' }, version: 0 })
  sessionStorage.setItem(options.storageKey, stored)
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new Error('storage denied')
  })
  const response = deferred<Response>()
  vi.stubGlobal('fetch', () => response.promise)
  const { result } = renderHook(() => useSession(options))
  expect(result.current.token).toBe('old-session')
  const oldRequest = result.current.api.me()
  act(() => result.current.changeToken('new-session'))
  const notice = result.current.notice
  await act(async () => {
    response.resolve(unauthorized())
    await expect(oldRequest).rejects.toBeInstanceOf(ApiError)
  })
  expect(sessionStorage.getItem(options.storageKey)).toBe(stored)
  expect(result.current.token).toBe('new-session')
  expect(result.current.notice).toBe(notice)
})

test('刷新后恢复令牌，持久化内容排除运行时状态', () => {
  const current = renderHook(() => useSession(options))
  act(() => current.result.current.changeToken('saved-session'))
  act(() => current.result.current.setNotice('本次运行提示'))
  const stored = sessionStorage.getItem(options.storageKey)
  expect(stored).not.toBeNull()
  expect(JSON.parse(stored || 'null')).toEqual({ state: { token: 'saved-session' }, version: 0 })
  current.unmount()

  const restored = renderHook(() => useSession(options))
  expect(restored.result.current.token).toBe('saved-session')
  expect(restored.result.current.version).toBe(0)
  expect(restored.result.current.notice).toBe('')
  expect(restored.result.current.storageNotice).toBe('')
})

test('企业和平台会话独立持久化，退出当前企业不能清理其他空间', () => {
  const enterprise = renderHook(() => useSession(options))
  const otherOptions = { storageKey: 'test.other.session' }
  const platformOptions = { storageKey: 'test.platform.session' }
  const other = renderHook(() => useSession(otherOptions))
  const platform = renderHook(() => useSession(platformOptions))
  act(() => enterprise.result.current.changeToken('enterprise-session'))
  act(() => other.result.current.changeToken('other-session'))
  act(() => platform.result.current.changeToken('platform-session'))
  act(() => enterprise.result.current.changeToken(''))
  act(() => enterprise.result.current.setNotice('已退出'))

  expect(enterprise.result.current.token).toBe('')
  expect(sessionStorage.getItem(options.storageKey)).toBeNull()
  expect(other.result.current.token).toBe('other-session')
  expect(platform.result.current.token).toBe('platform-session')
  other.unmount()
  platform.unmount()
  expect(renderHook(() => useSession(otherOptions)).result.current.token).toBe('other-session')
  expect(renderHook(() => useSession(platformOptions)).result.current.token).toBe('platform-session')
})

test('无法读取浏览器存储时保留错误提示，并允许在内存中登录', () => {
  vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('storage denied') })
  const { result } = renderHook(() => useSession(options))
  expect(result.current.token).toBe('')
  expect(result.current.storageNotice).toContain('读取')
  act(() => result.current.changeToken('memory-session'))
  expect(result.current.token).toBe('memory-session')
})

test('删除浏览器存储失败仍清空授权会话，并显示可处理的错误', () => {
  const { result } = renderHook(() => useSession(options))
  act(() => result.current.changeToken('saved-session'))
  vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => { throw new Error('storage denied') })
  act(() => result.current.changeToken(''))
  expect(result.current.token).toBe('')
  expect(result.current.storageNotice).toContain('清理会话')
})

test('业务提示不覆盖持久化失败，后续成功写入会清理存储错误', () => {
  const save = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('storage denied') })
  const { result } = renderHook(() => useSession(options))
  act(() => result.current.changeToken('memory-session'))
  act(() => result.current.setNotice('已登录'))
  expect(result.current.notice).toBe('已登录')
  expect(result.current.storageNotice).toContain('保存会话')
  save.mockRestore()
  act(() => result.current.changeToken('next-session'))
  expect(result.current.storageNotice).toBe('')
  expect(result.current.token).toBe('next-session')
})

test('更新业务提示时发生的持久化失败同样可见', () => {
  const { result } = renderHook(() => useSession(options))
  act(() => result.current.changeToken('saved-session'))
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('storage denied') })
  act(() => result.current.setNotice('操作已完成'))
  expect(result.current.notice).toBe('操作已完成')
  expect(result.current.storageNotice).toContain('保存会话')
  expect(result.current.token).toBe('saved-session')
})

test('StrictMode 初始化只读取记录，不写入或删除持久化会话', () => {
  const stored = JSON.stringify({ state: { token: 'saved-session' }, version: 0 })
  sessionStorage.setItem(options.storageKey, stored)
  const writes = vi.spyOn(Storage.prototype, 'setItem')
  const removes = vi.spyOn(Storage.prototype, 'removeItem')
  const { result } = renderHook(() => useSession(options), { wrapper: StrictMode })
  expect(result.current.token).toBe('saved-session')
  expect(sessionStorage.getItem(options.storageKey)).toBe(stored)
  expect(writes).not.toHaveBeenCalled()
  expect(removes).not.toHaveBeenCalled()
})

test('只从持久化会话恢复令牌，不恢复运行时版本、提示或操作', () => {
  sessionStorage.setItem(options.storageKey, JSON.stringify({
    state: { token: 'saved-session', version: 80, notice: '伪造提示', storageNotice: '伪造错误', changeToken: '伪造操作' },
    version: 0,
  }))
  const { result } = renderHook(() => useSession(options))
  expect(result.current.token).toBe('saved-session')
  expect(result.current.version).toBe(0)
  expect(result.current.notice).toBe('')
  expect(result.current.storageNotice).toBe('')
  act(() => result.current.changeToken('next-session'))
  expect(result.current.token).toBe('next-session')
})

test.each([
  'raw-session',
  '{bad-json',
  JSON.stringify({ state: { token: 12 }, version: 0 }),
  JSON.stringify({ state: { token: 'contains whitespace' }, version: 0 }),
  JSON.stringify({ state: { token: 'token\u0000value' }, version: 0 }),
  JSON.stringify({ state: { token: 'token\u007fvalue' }, version: 0 }),
  JSON.stringify({ state: { token: '中文令牌' }, version: 0 }),
  JSON.stringify({ state: { token: 'token=value' }, version: 0 }),
  JSON.stringify({ state: { token: 'x'.repeat(257) }, version: 0 }),
  JSON.stringify({ state: { token: 'saved-session' }, version: 7 }),
])('不把无效持久化记录当作可用登录令牌：%s', (stored) => {
  sessionStorage.setItem(options.storageKey, stored)
  const { result } = renderHook(() => useSession(options))
  expect(result.current.token).toBe('')
  expect(result.current.storageNotice).toMatch(/登录|读取/)
})

test.each([
  '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
  'N2JVh7uFjBfi_BqMLTp08mZyISnEwjDuKcG-uPSVOY',
  'a.B~c+d/e==',
])('恢复合法 Bearer 令牌时不误拒当前服务及协议允许的字符：%s', (token) => {
  sessionStorage.setItem(options.storageKey, JSON.stringify({ state: { token }, version: 0 }))
  const { result } = renderHook(() => useSession(options))
  expect(result.current.token).toBe(token)
  expect(result.current.storageNotice).toBe('')
})
