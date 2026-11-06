import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { AccountForms } from './AccountForms'
import { createAccountApi } from '../../../api/account/api'
import { createRequest } from '../../../api/request'
import { alice, bob, deferred } from '../../../../tests/fixtures/identity'
import type { User } from '../../../api/account/types'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

test('切换账号后昵称和密码草稿只属于新账号', () => {
  const api = createAccountApi(createRequest({}))
  const props = {
    api,
    onUserUpdated: vi.fn(),
    changePassword: api.password,
    onPasswordChanged: vi.fn(),
  }
  const { rerender } = render(<AccountForms {...props} user={alice} passwordTarget="tenant:2" />)
  fireEvent.change(screen.getByLabelText('昵称'), { target: { value: '旧身份未保存昵称' } })
  fireEvent.change(screen.getByLabelText('当前密码'), { target: { value: 'old-password' } })
  rerender(<AccountForms {...props} user={bob} passwordTarget="tenant:3" />)
  expect(screen.getByLabelText('昵称')).toHaveProperty('value', '小博')
  expect(screen.getByLabelText('当前密码')).toHaveProperty('value', '')
  expect(screen.getByLabelText('用户名', { exact: false })).toHaveProperty('value', 'bob')
})

test('切换账号后旧资料写入的延迟完成不会更新新账号', async () => {
  const response = deferred<Response>()
  vi.stubGlobal('fetch', () => response.promise)
  const api = createAccountApi(createRequest({}))
  const updated = vi.fn<(user: User) => void>()
  const props = {
    api,
    onUserUpdated: updated,
    changePassword: api.password,
    onPasswordChanged: vi.fn(),
  }
  const { rerender } = render(<AccountForms {...props} user={alice} passwordTarget="tenant:2" />)
  fireEvent.change(screen.getByLabelText('昵称'), { target: { value: '旧身份的新昵称' } })
  fireEvent.click(screen.getByRole('button', { name: '保存个人资料' }))
  rerender(<AccountForms {...props} user={bob} passwordTarget="tenant:3" />)
  await act(async () => {
    response.resolve(
      new Response(JSON.stringify({ data: { ...alice, nickname: '旧身份的新昵称' } })),
    )
    await response.promise
  })
  expect(updated).not.toHaveBeenCalled()
  expect(screen.getByLabelText('昵称')).toHaveProperty('value', '小博')
  expect(screen.queryByText('个人资料已保存。')).toBeNull()
})

test('没有企业用户的账号也按密码操作目标清理旧密码草稿', () => {
  const api = createAccountApi(createRequest({}))
  const props = {
    api,
    user: null,
    onUserUpdated: vi.fn(),
    changePassword: api.password,
    onPasswordChanged: vi.fn(),
  }
  const { rerender } = render(
    <AccountForms {...props} passwordTarget="platform:1" summary={<p>第一个账号摘要</p>} />,
  )
  fireEvent.change(screen.getByLabelText('当前密码'), { target: { value: 'old-password' } })
  rerender(<AccountForms {...props} passwordTarget="platform:2" summary={<p>第二个账号摘要</p>} />)
  expect(screen.getByLabelText('当前密码')).toHaveProperty('value', '')
  expect(screen.getByText('第二个账号摘要')).toBeTruthy()
  expect(screen.queryByText('第一个账号摘要')).toBeNull()
})
