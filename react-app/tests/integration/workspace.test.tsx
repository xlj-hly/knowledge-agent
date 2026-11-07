import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { WorkspaceRoutes } from '../../src/router/WorkspaceRoutes'
import { createApi } from '../../src/api/client'
import { alice, tenant, deferred } from '../fixtures/identity'
import { MemoryRouter, Route, Routes } from 'react-router'
import { aiFixture } from '../fixtures/ai'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

test('资料写入发出后离开页面，再返回会重新读取已生效的服务端资料', async () => {
  let stored = alice
  const pending = deferred<Response>()
  vi.stubGlobal('fetch', async (url: string) => {
    if (url.includes('/ai/') || url.endsWith('/knowledge/bases') || url.endsWith('/chat/sessions')) return new Response(JSON.stringify({ data: aiFixture(url) }))
    if (url.endsWith('/account/profile')) {
      // 服务端已提交，页面离开只取消等待，不能撤销已生效的数据。
      stored = { ...alice, nickname: '已经保存到服务端' }
      return pending.promise
    }
    return new Response(JSON.stringify({ data: { user: stored, tenant } }))
  })
  const api = createApi({ token: 'session' })
  render(
    <MemoryRouter><WorkspaceRoutes
      api={api}
      platform={false}
      identity={{ user: alice, tenant }}
      onSessionEnd={vi.fn()}
    /></MemoryRouter>,
  )
  fireEvent.click(screen.getByRole('link', { name: '打开个人账号' }))
  fireEvent.change(await screen.findByLabelText('昵称'), { target: { value: '已经保存到服务端' } })
  fireEvent.click(screen.getByRole('button', { name: '保存个人资料' }))
  fireEvent.click(screen.getByRole('link', { name: /^对话$/ }))
  fireEvent.click(screen.getByRole('link', { name: '打开个人账号' }))
  expect(await screen.findByLabelText('昵称')).toHaveProperty('value', '已经保存到服务端')
  await act(async () => {
    pending.resolve(new Response(JSON.stringify({ data: stored })))
    await pending.promise
  })
  expect(screen.queryByText('个人资料已保存。')).toBeNull()
  expect(screen.getByLabelText('昵称')).toHaveProperty('value', '已经保存到服务端')
})

test('应用组装平台账号摘要，并将改密提交到平台身份', async () => {
  const admin = { id: '7', username: 'platform_root', created_at: '2026-11-05T00:00:00Z' }
  const writes: { path: string; body: unknown }[] = []
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (options.method === 'POST') {
      writes.push({ path: url, body: JSON.parse(String(options.body)) })
      return new Response(JSON.stringify({ data: { ok: true } }))
    }
    return new Response(JSON.stringify({ data: { admin } }))
  })
  const api = createApi({ token: 'platform-session' })
  const endSession = vi.fn()
  render(<MemoryRouter initialEntries={['/platform']}><Routes><Route path="/platform/*" element={<WorkspaceRoutes api={api} platform={true} identity={{ admin }} onSessionEnd={endSession} />} /></Routes></MemoryRouter>)
  fireEvent.click(screen.getByRole('link', { name: '打开个人账号' }))
  expect(await screen.findByRole('heading', { name: '平台管理员' })).toBeTruthy()
  expect(screen.getByRole('heading', { name: 'platform_root' })).toBeTruthy()
  expect(screen.getByText('账号创建于').nextElementSibling?.textContent).toBe('2026/11/05')
  expect(screen.queryByLabelText('昵称')).toBeNull()
  fireEvent.change(screen.getByLabelText('当前密码'), { target: { value: 'old-password' } })
  fireEvent.change(screen.getByLabelText(/^新密码/), { target: { value: 'new-password' } })
  fireEvent.change(screen.getByLabelText('确认新密码'), { target: { value: 'new-password' } })
  fireEvent.click(screen.getByRole('button', { name: '更新密码并重新登录' }))
  await waitFor(() => expect(endSession).toHaveBeenCalledWith('密码已更新，请使用新密码重新登录。'))
  expect(writes).toEqual([
    {
      path: '/api/platform/account/password',
      body: { current_password: 'old-password', new_password: 'new-password' },
    },
  ])
})
