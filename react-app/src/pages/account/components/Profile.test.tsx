import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { createAccountApi } from '../../../api/account/api'
import { createRequest } from '../../../api/request'
import { alice, deferred } from '../../../../tests/fixtures/identity'
import { Profile } from './Profile'

const avatarUrl = `/api/avatars/${alice.tenant_id}/${alice.id}/abc123.png`
const response = (data: unknown) => new Response(JSON.stringify({ data }))
const file = () => new File(['image'], 'portrait.png', { type: 'image/png' })

afterEach(() => { cleanup(); vi.unstubAllGlobals() })

test('选择图片后先带认证上传 multipart，保存受控相对地址；恢复资料后改昵称保留头像', async () => {
  const writes: { path: string; body: unknown }[] = []
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    expect(options.headers).toHaveProperty('Authorization', 'Bearer member-session')
    if (url.endsWith('/avatar')) {
      expect(options.headers).not.toHaveProperty('Content-Type')
      expect(options.body).toBeInstanceOf(FormData)
      writes.push({ path: url, body: options.body })
      return response({ avatar_url: avatarUrl })
    }
    const body = JSON.parse(String(options.body))
    writes.push({ path: url, body })
    return response({ ...alice, ...body })
  })
  const api = createAccountApi(createRequest({ token: 'member-session' }))
  const updated = vi.fn()
  const view = render(<Profile api={api} user={alice} onUpdated={updated} />)
  fireEvent.change(screen.getByLabelText('选择头像', { exact: false }), { target: { files: [file()] } })
  fireEvent.change(screen.getByLabelText('昵称'), { target: { value: '新昵称' } })
  fireEvent.click(screen.getByRole('button', { name: '保存个人资料' }))
  await waitFor(() => expect(updated).toHaveBeenCalledWith({ ...alice, nickname: '新昵称', avatar_url: avatarUrl }))
  expect(writes).toHaveLength(2)
  expect(writes[1]).toEqual({ path: '/api/account/profile', body: { nickname: '新昵称', avatar_url: avatarUrl } })
  view.unmount()
  render(<Profile api={api} user={{ ...alice, avatar_url: avatarUrl }} onUpdated={updated} />)
  fireEvent.change(screen.getByLabelText('昵称'), { target: { value: '再次改昵称' } })
  fireEvent.click(screen.getByRole('button', { name: '保存个人资料' }))
  await waitFor(() => expect(writes).toHaveLength(3))
  expect(writes[2]).toEqual({ path: '/api/account/profile', body: { nickname: '再次改昵称', avatar_url: avatarUrl } })
})

test('无效或超限图片给出提示，上传失败不提交资料且可以重试', async () => {
  let attempts = 0
  const profile = vi.fn()
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (url.endsWith('/avatar')) {
      attempts += 1
      if (attempts === 1) return new Response(JSON.stringify({ error: { code: 'storage_error', message: '头像上传失败，请重试' } }), { status: 500 })
      return response({ avatar_url: avatarUrl })
    }
    profile()
    return response({ ...alice, ...JSON.parse(String(options.body)) })
  })
  render(<Profile api={createAccountApi(createRequest({}))} user={alice} onUpdated={vi.fn()} />)
  const input = screen.getByLabelText('选择头像', { exact: false })
  fireEvent.change(input, { target: { files: [new File(['text'], 'x.svg', { type: 'image/svg+xml' })] } })
  expect(screen.getByRole('alert').textContent).toContain('JPG 或 PNG')
  fireEvent.change(input, { target: { files: [new File([new Uint8Array(2 * 1024 * 1024 + 1)], 'large.png', { type: 'image/png' })] } })
  expect(screen.getByRole('alert').textContent).toContain('2MB')
  fireEvent.change(input, { target: { files: [file()] } })
  fireEvent.click(screen.getByRole('button', { name: '保存个人资料' }))
  expect((await screen.findByRole('alert')).textContent).toContain('头像上传失败')
  expect(profile).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: '保存个人资料' }))
  await waitFor(() => expect(profile).toHaveBeenCalledOnce())
})

test('离开账号时取消上传，迟到的上传响应不能再发资料写入或更新其他身份', async () => {
  const pending = deferred<Response>()
  const paths: string[] = []
  let signal: AbortSignal | null | undefined
  vi.stubGlobal('fetch', (url: string, options: RequestInit) => { paths.push(url); signal = options.signal; return pending.promise })
  const updated = vi.fn()
  const view = render(<Profile api={createAccountApi(createRequest({}))} user={alice} onUpdated={updated} />)
  fireEvent.change(screen.getByLabelText('选择头像', { exact: false }), { target: { files: [file()] } })
  fireEvent.click(screen.getByRole('button', { name: '保存个人资料' }))
  await waitFor(() => expect(paths).toEqual(['/api/account/avatar']))
  view.unmount()
  expect(signal?.aborted).toBe(true)
  await act(async () => pending.resolve(response({ avatar_url: avatarUrl })))
  expect(paths).toEqual(['/api/account/avatar'])
  expect(updated).not.toHaveBeenCalled()
})
