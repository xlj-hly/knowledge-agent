import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { createApi } from '../../../../api/client'
import { deferred, tenant } from '../../../../../tests/fixtures/identity'
import { WechatForm } from './WechatForm'

const appId = 'wx1234567890abcdef'
const secondAppId = 'wxabcdef1234567890'
const dataResponse = (data: unknown) => new Response(JSON.stringify({ data }))

// jsdom 不提供原生 dialog 的展示 API；这里只补齐浏览器能力，不替换表单交互。
beforeEach(() => {
  Object.defineProperty(HTMLDialogElement.prototype, 'showModal', { configurable: true, value() { this.setAttribute('open', '') } })
  Object.defineProperty(HTMLDialogElement.prototype, 'close', { configurable: true, value() { this.removeAttribute('open') } })
})

afterEach(() => { cleanup(); vi.unstubAllGlobals(); sessionStorage.clear(); localStorage.clear() })

test('读取已配置的小程序，密钥不回填；留空保存只发送 AppID，服务确认后才通知完成', async () => {
  const pending = deferred<Response>()
  const writes: unknown[] = []
  vi.stubGlobal('fetch', async (_url: string, options: RequestInit) => {
    if (options.method === 'PUT') { writes.push(JSON.parse(String(options.body))); return pending.promise }
    return dataResponse({ app_id: appId, has_secret: true })
  })
  const saved = vi.fn()
  render(<WechatForm api={createApi({ token: 'platform-session' })} tenant={tenant} onClose={vi.fn()} onSaved={saved} />)
  expect(await screen.findByLabelText('小程序 AppID')).toHaveProperty('value', appId)
  expect(screen.getByLabelText('小程序 AppSecret', { exact: false })).toHaveProperty('value', '')
  fireEvent.click(screen.getByRole('button', { name: '保存小程序配置' }))
  await waitFor(() => expect(writes).toEqual([{ app_id: appId }]))
  expect(saved).not.toHaveBeenCalled()
  expect(screen.getByRole('button', { name: '关闭弹窗' })).toHaveProperty('disabled', true)
  await act(async () => pending.resolve(dataResponse({ app_id: appId, has_secret: true })))
  await waitFor(() => expect(saved).toHaveBeenCalledOnce())
})

test('首次配置与更换 AppID 都要求新密钥，填写后提交，密码草稿不进入持久化', async () => {
  const writes: unknown[] = []
  vi.stubGlobal('fetch', async (_url: string, options: RequestInit) => {
    if (options.method === 'PUT') writes.push(JSON.parse(String(options.body)))
    return dataResponse({ app_id: appId, has_secret: true })
  })
  render(<WechatForm api={createApi({})} tenant={tenant} onClose={vi.fn()} onSaved={vi.fn()} />)
  fireEvent.change(await screen.findByLabelText('小程序 AppID'), { target: { value: secondAppId } })
  fireEvent.click(screen.getByRole('button', { name: '保存小程序配置' }))
  expect((await screen.findByRole('alert')).textContent).toContain('更换 AppID')
  expect(writes).toHaveLength(0)
  fireEvent.change(screen.getByLabelText('小程序 AppSecret', { exact: false }), { target: { value: 'test-only-secret' } })
  fireEvent.click(screen.getByRole('button', { name: '保存小程序配置' }))
  await waitFor(() => expect(writes).toEqual([{ app_id: secondAppId, app_secret: 'test-only-secret' }]))
  await waitFor(() => expect(screen.getByLabelText('小程序 AppSecret', { exact: false })).toHaveProperty('value', ''))
  expect(sessionStorage.length).toBe(0)
  expect(localStorage.length).toBe(0)
})

test('首次配置没有密钥不能提交，失败可显示原因并更正重试', async () => {
  let attempts = 0
  vi.stubGlobal('fetch', async (_url: string, options: RequestInit) => {
    if (options.method !== 'PUT') return dataResponse({ app_id: '', has_secret: false })
    attempts += 1
    if (attempts === 1) return new Response(JSON.stringify({ error: { code: 'invalid_input', message: '密钥格式不正确' } }), { status: 400 })
    return dataResponse({ app_id: appId, has_secret: true })
  })
  const saved = vi.fn()
  render(<WechatForm api={createApi({})} tenant={tenant} onClose={vi.fn()} onSaved={saved} />)
  fireEvent.change(await screen.findByLabelText('小程序 AppID'), { target: { value: appId } })
  fireEvent.click(screen.getByRole('button', { name: '保存小程序配置' }))
  expect((await screen.findByRole('alert')).textContent).toContain('首次配置')
  expect(attempts).toBe(0)
  fireEvent.change(screen.getByLabelText('小程序 AppSecret', { exact: false }), { target: { value: 'first-test-secret' } })
  fireEvent.click(screen.getByRole('button', { name: '保存小程序配置' }))
  await waitFor(() => expect(screen.getByRole('alert').textContent).toContain('密钥格式不正确'))
  expect(saved).not.toHaveBeenCalled()
  fireEvent.change(screen.getByLabelText('小程序 AppSecret', { exact: false }), { target: { value: 'replacement-test-secret' } })
  fireEvent.click(screen.getByRole('button', { name: '保存小程序配置' }))
  await waitFor(() => expect(saved).toHaveBeenCalledOnce())
})

test('配置读取失败可重新加载，切换租户取消旧读取，旧响应不能覆盖新租户', async () => {
  const pending = deferred<Response>()
  let firstAttempts = 0
  let oldSignal: AbortSignal | null | undefined
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (url.includes(tenant.id)) {
      firstAttempts += 1
      if (firstAttempts === 1) throw new Error('offline')
      oldSignal = options.signal
      return pending.promise
    }
    return dataResponse({ app_id: secondAppId, has_secret: false })
  })
  const api = createApi({})
  const props = { api, onClose: vi.fn(), onSaved: vi.fn() }
  const view = render(<WechatForm {...props} tenant={tenant} />)
  expect((await screen.findByRole('alert')).textContent).toContain('无法连接服务')
  fireEvent.click(screen.getByRole('button', { name: '重新加载' }))
  await waitFor(() => expect(firstAttempts).toBe(2))
  view.rerender(<WechatForm {...props} tenant={{ ...tenant, id: '88', name: '另一企业' }} />)
  expect(await screen.findByLabelText('小程序 AppID')).toHaveProperty('value', secondAppId)
  expect(oldSignal?.aborted).toBe(true)
  await act(async () => pending.resolve(dataResponse({ app_id: appId, has_secret: true })))
  expect(screen.getByLabelText('小程序 AppID')).toHaveProperty('value', secondAppId)
  expect(screen.getByLabelText('小程序 AppSecret', { exact: false })).toHaveProperty('value', '')
})

test('切换身份取消旧配置读取并丢弃密钥草稿，离开后旧写入不能发布完成', async () => {
  const pending = deferred<Response>()
  let writeSignal: AbortSignal | null | undefined
  vi.stubGlobal('fetch', async (_url: string, options: RequestInit) => {
    if (options.method === 'PUT') { writeSignal = options.signal; return pending.promise }
    return dataResponse({ app_id: appId, has_secret: true })
  })
  const saved = vi.fn()
  const view = render(<WechatForm api={createApi({ token: 'old-session' })} tenant={tenant} onClose={vi.fn()} onSaved={saved} />)
  fireEvent.change(await screen.findByLabelText('小程序 AppSecret', { exact: false }), { target: { value: 'discarded-test-secret' } })
  fireEvent.click(screen.getByRole('button', { name: '保存小程序配置' }))
  await waitFor(() => expect(writeSignal).toBeDefined())
  view.unmount()
  expect(writeSignal?.aborted).toBe(true)
  await act(async () => pending.resolve(dataResponse({ app_id: appId, has_secret: true })))
  expect(saved).not.toHaveBeenCalled()
  render(<WechatForm api={createApi({ token: 'new-session' })} tenant={tenant} onClose={vi.fn()} onSaved={vi.fn()} />)
  expect(await screen.findByLabelText('小程序 AppSecret', { exact: false })).toHaveProperty('value', '')
})
