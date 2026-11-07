import { useState } from 'react'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { MemoryRouter } from 'react-router'
import { ChatComposer } from './ChatComposer'

afterEach(() => { cleanup(); vi.unstubAllGlobals() })

function Composer({ streaming = false, onSend, onStop = () => {} }: { streaming?: boolean; onSend: () => void; onStop?: () => void }) {
  const [input, setInput] = useState('')
  return <MemoryRouter><ChatComposer input={input} onInput={setInput} onSend={onSend} onStop={onStop} loading={false} streaming={streaming} bases={[]} selected={[]} onSelect={() => {}} /></MemoryRouter>
}

test('Shift 换行和中文输入法确认不触发发送，普通 Enter 才发送', () => {
  const onSend = vi.fn()
  render(<Composer onSend={onSend} />)
  const input = screen.getByRole('textbox', { name: '输入消息' })
  fireEvent.change(input, { target: { value: '你好' } })
  fireEvent.keyDown(input, { key: 'Enter', shiftKey: true })
  fireEvent.keyDown(input, { key: 'Enter', isComposing: true })
  fireEvent.keyDown(input, { key: 'Enter', keyCode: 229 })
  fireEvent.compositionStart(input)
  fireEvent.keyDown(input, { key: 'Enter' })
  expect(onSend).not.toHaveBeenCalled()
  expect(input).toHaveProperty('value', '你好')
  fireEvent.compositionEnd(input)
  fireEvent.keyDown(input, { key: 'Enter' })
  expect(onSend).toHaveBeenCalledTimes(1)
})

test('生成中保留下一问草稿且 Enter 或表单提交不能重复发送，停止后可继续发送', () => {
  const onSend = vi.fn()
  const onStop = vi.fn()
  const view = render(<Composer streaming onSend={onSend} onStop={onStop} />)
  const input = screen.getByRole('textbox', { name: '输入消息' })
  expect(input).toHaveProperty('disabled', false)
  fireEvent.change(input, { target: { value: '我的下一问' } })
  fireEvent.keyDown(input, { key: 'Enter' })
  const form = input.closest('form')
  if (!form) throw new Error('输入框必须属于对话发送表单')
  fireEvent.submit(form)
  expect(onSend).not.toHaveBeenCalled()
  expect(screen.queryByRole('button', { name: '发送' })).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: '停止生成' }))
  expect(onStop).toHaveBeenCalledTimes(1)
  expect(input).toHaveProperty('value', '我的下一问')
  view.rerender(<Composer streaming={false} onSend={onSend} onStop={onStop} />)
  expect(input).toHaveProperty('value', '我的下一问')
  fireEvent.click(screen.getByRole('button', { name: '发送' }))
  expect(onSend).toHaveBeenCalledTimes(1)
})

test('空白消息不能通过按键或表单提交发送', () => {
  const onSend = vi.fn()
  render(<Composer onSend={onSend} />)
  const input = screen.getByRole('textbox', { name: '输入消息' })
  fireEvent.change(input, { target: { value: '  \n  ' } })
  expect(screen.getByRole('button', { name: '发送' })).toHaveProperty('disabled', true)
  fireEvent.keyDown(input, { key: 'Enter' })
  const form = input.closest('form')
  if (!form) throw new Error('输入框必须属于对话发送表单')
  fireEvent.submit(form)
  expect(onSend).not.toHaveBeenCalled()
})
