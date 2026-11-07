import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { createApi } from '../../../../api/client'
import { deferred, tenant } from '../../../../../tests/fixtures/identity'
import { WechatForm } from './WechatForm'

const appId = 'wx1234567890abcdef'
const secondAppId = 'wxabcdef1234567890'
const appSecret = 'fixture-wechat-secret'
const dataResponse = (data: unknown) => new Response(JSON.stringify({ data }))

// jsdom 不提供原生 dialog 的展示 API；这里只补齐浏览器能力，不替换表单交互。
beforeEach(() => {
  Object.defineProperty(HTMLDialogElement.prototype, 'showModal', { configurable: true, value() { this.setAttribute('open', '') } })
  Object.defineProperty(HTMLDialogElement.prototype, 'close', { configurable: true, value() { this.removeAttribute('open') } })
})

afterEach(() => { cleanup(); vi.unstubAllGlobals(); sessionStorage.clear(); localStorage.clear() })

test('读取完整小程序配置并回填，保存提交完整字段且服务确认后才通知完成', async () => {
  const pending = deferred<Response>()
  const writes: unknown[] = []
  vi.stubGlobal('fetch', async (_url: string, options: RequestInit) => {
    if (options.method === 'PUT') { writes.push(JSON.parse(String(options.body))); return pending.promise }
    return dataResponse({ app_id: appId, app_secret: appSecret })
  })
  const saved = vi.fn()
  render(<WechatForm api={createApi({ token: 'platform-session' })} tenant={tenant} onClose={vi.fn()} onSaved={saved} />)
  expect(await screen.findByLabelText('小程序 AppID')).toHaveProperty('value', appId)
  expect(screen.getByLabelText('小程序 AppSecret', { exact: false })).toHaveProperty('value', appSecret)
  fireEvent.click(screen.getByRole('button', { name: '保存小程序配置' }))
  await waitFor(() => expect(writes).toEqual([{ app_id: appId, app_secret: appSecret }]))
  expect(saved).not.toHaveBeenCalled()
  expect(screen.getByRole('button', { name: '关闭弹窗' })).toHaveProperty('disabled', true)
  await act(async () => pending.resolve(dataResponse({ app_id: appId, app_secret: appSecret })))
  await waitFor(() => expect(saved).toHaveBeenCalledOnce())
  expect(screen.getByLabelText('小程序 AppSecret', { exact: false })).toHaveProperty('value', appSecret)
})

test('更换 AppID 保留表单凭据，修改后完整提交，已保存字段不清空也不进入本地存储', async () => {
  const writes: unknown[] = []
  vi.stubGlobal('fetch', async (_url: string, options: RequestInit) => {
    if (options.method === 'PUT') writes.push(JSON.parse(String(options.body)))
    return dataResponse(options.method === 'PUT' ? { app_id: secondAppId, app_secret: 'test-only-secret' } : { app_id: appId, app_secret: appSecret })
  })
  render(<WechatForm api={createApi({})} tenant={tenant} onClose={vi.fn()} onSaved={vi.fn()} />)
  fireEvent.change(await screen.findByLabelText('小程序 AppID'), { target: { value: secondAppId } })
  expect(screen.getByLabelText('小程序 AppSecret', { exact: false })).toHaveProperty('value', appSecret)
  fireEvent.change(screen.getByLabelText('小程序 AppSecret', { exact: false }), { target: { value: 'test-only-secret' } })
  fireEvent.click(screen.getByRole('button', { name: '保存小程序配置' }))
  await waitFor(() => expect(writes).toEqual([{ app_id: secondAppId, app_secret: 'test-only-secret' }]))
  await waitFor(() => expect(screen.getByLabelText('小程序 AppSecret', { exact: false })).toHaveProperty('value', 'test-only-secret'))
  expect(sessionStorage.length).toBe(0)
  expect(localStorage.length).toBe(0)
})

test('首次配置没有密钥不能提交，失败可显示原因并更正重试', async () => {
  let attempts = 0
  vi.stubGlobal('fetch', async (_url: string, options: RequestInit) => {
    if (options.method !== 'PUT') return dataResponse({ app_id: '', app_secret: '' })
    attempts += 1
    if (attempts === 1) return new Response(JSON.stringify({ error: { code: 'invalid_input', message: '密钥格式不正确' } }), { status: 400 })
    return dataResponse({ app_id: appId, app_secret: 'replacement-test-secret' })
  })
  const saved = vi.fn()
  render(<WechatForm api={createApi({})} tenant={tenant} onClose={vi.fn()} onSaved={saved} />)
  fireEvent.change(await screen.findByLabelText('小程序 AppID'), { target: { value: appId } })
  fireEvent.click(screen.getByRole('button', { name: '保存小程序配置' }))
  expect((await screen.findByRole('alert')).textContent).toContain('请填写小程序 AppSecret')
  expect(attempts).toBe(0)
  fireEvent.change(screen.getByLabelText('小程序 AppSecret', { exact: false }), { target: { value: 'first-test-secret' } })
  fireEvent.click(screen.getByRole('button', { name: '保存小程序配置' }))
  await waitFor(() => expect(screen.getByRole('alert').textContent).toContain('密钥格式不正确'))
  expect(saved).not.toHaveBeenCalled()
  expect(screen.getByLabelText('小程序 AppSecret', { exact: false })).toHaveProperty('value', 'first-test-secret')
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
    return dataResponse({ app_id: secondAppId, app_secret: 'second-wechat-secret' })
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
  await act(async () => pending.resolve(dataResponse({ app_id: appId, app_secret: appSecret })))
  expect(screen.getByLabelText('小程序 AppID')).toHaveProperty('value', secondAppId)
  expect(screen.getByLabelText('小程序 AppSecret', { exact: false })).toHaveProperty('value', 'second-wechat-secret')
})

test('切换身份取消旧配置读取并丢弃密钥草稿，离开后旧写入不能发布完成', async () => {
  const pending = deferred<Response>()
  let writeSignal: AbortSignal | null | undefined
  vi.stubGlobal('fetch', async (_url: string, options: RequestInit) => {
    if (options.method === 'PUT') { writeSignal = options.signal; return pending.promise }
    return dataResponse({ app_id: appId, app_secret: appSecret })
  })
  const saved = vi.fn()
  const view = render(<WechatForm api={createApi({ token: 'old-session' })} tenant={tenant} onClose={vi.fn()} onSaved={saved} />)
  fireEvent.change(await screen.findByLabelText('小程序 AppSecret', { exact: false }), { target: { value: 'discarded-test-secret' } })
  fireEvent.click(screen.getByRole('button', { name: '保存小程序配置' }))
  await waitFor(() => expect(writeSignal).toBeDefined())
  view.unmount()
  expect(writeSignal?.aborted).toBe(true)
  await act(async () => pending.resolve(dataResponse({ app_id: appId, app_secret: appSecret })))
  expect(saved).not.toHaveBeenCalled()
  render(<WechatForm api={createApi({ token: 'new-session' })} tenant={tenant} onClose={vi.fn()} onSaved={vi.fn()} />)
  expect(await screen.findByLabelText('小程序 AppSecret', { exact: false })).toHaveProperty('value', appSecret)
})

test('已配置密钥被清空时校验拒绝提交，不再沿用服务端旧值', async () => {
  const writes: unknown[] = []
  vi.stubGlobal('fetch', async (_url: string, options: RequestInit) => {
    if (options.method === 'PUT') writes.push(JSON.parse(String(options.body)))
    return dataResponse({ app_id: appId, app_secret: appSecret })
  })
  render(<WechatForm api={createApi({})} tenant={tenant} onClose={vi.fn()} onSaved={vi.fn()} />)
  fireEvent.change(await screen.findByLabelText('小程序 AppSecret', { exact: false }), { target: { value: '' } })
  fireEvent.click(screen.getByRole('button', { name: '保存小程序配置' }))
  expect((await screen.findByRole('alert')).textContent).toContain('请填写小程序 AppSecret')
  expect(writes).toHaveLength(0)
})
