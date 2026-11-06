import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { LoginPage } from './LoginPage'
import { createAuthApi } from '../../api/auth/api'
import { createRequest } from '../../api/request'
import { alice, tenant } from '../../../tests/fixtures/identity'
import { MemoryRouter } from 'react-router'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

test('登录失败后可更正并进入域名对应企业', async () => {
  let loginRequests = 0
  vi.stubGlobal('fetch', async () => {
    loginRequests += 1
    if (loginRequests === 1)
      return new Response(
        JSON.stringify({ error: { code: 'unauthorized', message: '用户名或密码错误' } }),
        { status: 401 },
      )
    return new Response(
      JSON.stringify({
        data: { token: 'new-session', expires_at: '2026-11-12T00:00:00Z', user: alice, tenant },
      }),
    )
  })
  const api = createAuthApi(createRequest({}))
  const login = vi.fn()
  render(<MemoryRouter><LoginPage api={api} tenant={tenant} platform={false} homePath="/" notice="" onLogin={login} /></MemoryRouter>)
  fireEvent.change(await screen.findByLabelText('用户名'), { target: { value: 'alice' } })
  fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'wrong-password' } })
  fireEvent.click(screen.getByRole('button', { name: '登录工作空间' }))
  expect(await screen.findByRole('alert')).toHaveProperty(
    'textContent',
    expect.stringContaining('用户名或密码错误'),
  )
  expect(login).not.toHaveBeenCalled()
  fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'example-password' } })
  fireEvent.click(screen.getByRole('button', { name: '登录工作空间' }))
  await waitFor(() => expect(login).toHaveBeenCalledWith('new-session'))
})
