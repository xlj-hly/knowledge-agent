import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { createApi } from '../../../api/client'
import { useKnowledgeDetail } from './useKnowledgeDetail'
import { knowledgeBase, knowledgeDocument } from '../../../../tests/fixtures/knowledge'

afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.useRealTimers() })
test('离开知识库中止正在进行的轮询并清理下一次计时器', async () => {
  vi.useFakeTimers()
  let requests = 0
  const signals: (AbortSignal | null | undefined)[] = []
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    requests += 1
    signals.push(options.signal)
    if (requests > 2) return new Promise<Response>((_resolve, reject) => {
      options.signal?.addEventListener('abort', () => reject(new DOMException('已取消', 'AbortError')), { once: true })
    })
    return new Response(JSON.stringify({ data: url.endsWith('/documents') ? [{ ...knowledgeDocument, parse_status: 'queued' }] : knowledgeBase }))
  })
  const api = createApi({})
  const view = renderHook(() => useKnowledgeDetail(api, '42'))
  await act(async () => { await vi.advanceTimersByTimeAsync(0) })
  expect(view.result.current.snapshot?.documents[0].parse_status).toBe('queued')
  expect(requests).toBe(2)
  await act(async () => { await vi.advanceTimersByTimeAsync(1500) })
  expect(requests).toBe(4)
  view.unmount()
  expect(signals.slice(2).every(signal => signal?.aborted)).toBe(true)
  await act(async () => { await vi.advanceTimersByTimeAsync(5000) })
  expect(requests).toBe(4)
})

test('刷新失败保留已展示的资料，显示原因并允许再次刷新恢复', async () => {
  let unavailable = false
  vi.stubGlobal('fetch', async (url: string) => {
    if (unavailable) throw new Error('offline')
    return new Response(JSON.stringify({ data: url.endsWith('/documents') ? [knowledgeDocument] : knowledgeBase }))
  })
  const api = createApi({})
  const view = renderHook(() => useKnowledgeDetail(api, '42'))
  await waitFor(() => expect(view.result.current.snapshot?.base.name).toBe('员工手册'))
  unavailable = true
  act(() => view.result.current.refresh())
  await waitFor(() => expect(view.result.current.error).toContain('无法连接服务'))
  expect(view.result.current.snapshot?.documents[0].title).toBe('休假制度')
  unavailable = false
  act(() => view.result.current.refresh())
  await waitFor(() => expect(view.result.current.error).toBe(''))
})
