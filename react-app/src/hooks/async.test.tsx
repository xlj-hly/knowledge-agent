import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { useAction } from './async'

afterEach(cleanup)

test('旧身份保留的提交回调不能在新身份下启动写入', async () => {
  const { result, rerender } = renderHook(({ scope }) => useAction(scope), {
    initialProps: { scope: 'old-session' },
  })
  const oldRun = result.current.run
  rerender({ scope: 'new-session' })
  const write = vi.fn(async () => 'saved')
  await act(() => oldRun(write))
  expect(write).not.toHaveBeenCalled()
  expect(result.current.busy).toBe(false)
})

test('身份切换后旧写入即使完成，也不能改变新页面或触发成功动作', async () => {
  let complete: (value: string) => void = () => {
    throw new Error('未启动写入')
  }
  const pending = new Promise<string>((resolve) => {
    complete = resolve
  })
  const saved = vi.fn()
  const { result, rerender } = renderHook(({ scope }) => useAction(scope), {
    initialProps: { scope: 'old-session' },
  })
  let task: Promise<void> = Promise.resolve()
  act(() => {
    task = result.current.run(() => pending, saved)
  })
  rerender({ scope: 'new-session' })
  await act(async () => {
    complete('saved')
    await task
  })
  expect(saved).not.toHaveBeenCalled()
  expect(result.current.busy).toBe(false)
  expect(result.current.error).toBe('')
})
