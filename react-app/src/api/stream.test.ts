import { afterEach, expect, test, vi } from 'vitest'
import { createStreamRequest } from './stream'

afterEach(() => vi.unstubAllGlobals())

test('流式请求复用身份并正确处理跨字节的中文与多个 SSE 事件', async () => {
  const bytes = new TextEncoder().encode('event: delta\ndata: {"content":"你好"}\n\nevent: done\ndata: {"ok":true}\n\n')
  const fetcher = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response(new ReadableStream({ start(controller) {
    controller.enqueue(bytes.slice(0, 36)); controller.enqueue(bytes.slice(36)); controller.close()
  } }), { headers: { 'Content-Type': 'text/event-stream' } }))
  vi.stubGlobal('fetch', fetcher)
  const events: unknown[] = []
  await createStreamRequest({ token: 'test-token', baseUrl: '/api' })('/chat/sessions/1/messages', { content: '问题' }, (event, value) => events.push({ event, value }))
  expect(events).toEqual([{ event: 'delta', value: { content: '你好' } }, { event: 'done', value: { ok: true } }])
  expect(fetcher.mock.calls[0]?.[1]).toMatchObject({ headers: { Authorization: 'Bearer test-token' } })
})

test('流式鉴权失败使会话失效，业务错误保留服务端说明', async () => {
  const unauthorized = vi.fn()
  vi.stubGlobal('fetch', async () => new Response(JSON.stringify({ error: { code: 'unauthorized', message: '请重新登录' } }), { status: 401 }))
  await expect(createStreamRequest({ token: 'token', onUnauthorized: unauthorized })('/chat', {}, () => {})).rejects.toThrow('请重新登录')
  expect(unauthorized).toHaveBeenCalledOnce()
})
