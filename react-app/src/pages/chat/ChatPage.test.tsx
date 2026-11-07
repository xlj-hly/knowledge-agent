import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { createMemoryRouter, MemoryRouter, Route, RouterProvider, Routes } from 'react-router'
import { createApi } from '../../api/client'
import type { ChatSession, Message } from '../../api/chat/types'
import { ChatPage } from './ChatPage'
import { conversationDefaults } from './hooks/useConversation'
import { aiFixture, aiSettings, chatModel } from '../../../tests/fixtures/ai'
import { knowledgeBase } from '../../../tests/fixtures/knowledge'
import { deferred } from '../../../tests/fixtures/identity'

const response = (data: unknown) => new Response(JSON.stringify({ data }))
const session: ChatSession = { ...conversationDefaults({ ...aiSettings, chat_model_id: chatModel.id }), id: '55', created_at: '2026-11-06T00:00:00Z', updated_at: '2026-11-06T00:00:00Z' }
const source = { base_id: '42', document_id: '71', chunk_id: '81', document_title: '航星审批规则', content: '航星计划的审批人是林沐。', score: 0.8 }
const assistant: Message = { id: '102', session_id: '55', role: 'assistant', content: '', status: 'generating', sources: [], created_at: '2026-11-06T00:00:00Z' }
beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
  const computedStyle = window.getComputedStyle.bind(window)
  vi.stubGlobal('getComputedStyle', (element: Element) => computedStyle(element))
  Object.defineProperty(HTMLDialogElement.prototype, 'showModal', { configurable: true, value() { this.setAttribute('open', '') } })
  Object.defineProperty(HTMLDialogElement.prototype, 'close', { configurable: true, value() { this.removeAttribute('open') } })
})
afterEach(() => { cleanup(); vi.unstubAllGlobals() })
function boot(path = '/') { return render(<MemoryRouter initialEntries={[path]}><Routes><Route path="/" element={<ChatPage api={api} />} /><Route path="/chat/:id" element={<ChatPage api={api} />} /></Routes></MemoryRouter>) }
const api = createApi({ token: 'tenant-token' })

test('聊天不提供模型选择，Enter发送，Shift和中文输入法回车不发送', async () => {
  let sends = 0
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (url.endsWith('/messages') && options.method === 'POST') {
      sends += 1
      return new Response(`event: done\ndata: ${JSON.stringify({ ...assistant, status: 'completed', content: '**完成**' })}\n\n`, { headers: { 'Content-Type': 'text/event-stream' } })
    }
    if (url.endsWith('/chat/sessions') && options.method === 'POST') return response(session)
    if (url.endsWith('/chat/sessions/55/messages')) return response([])
    if (url.endsWith('/chat/sessions/55')) return response(session)
    if (url.endsWith('/ai/models')) return response([chatModel])
    if (url.endsWith('/ai/settings')) return response({ ...aiSettings, chat_model_id: chatModel.id })
    return response(aiFixture(url))
  })
  boot()
  const input = await screen.findByLabelText('输入消息')
  expect(screen.queryByLabelText('对话模型')).toBeNull()
  fireEvent.change(input, { target: { value: '你好' } })
  fireEvent.keyDown(input, { key: 'Enter', shiftKey: true })
  fireEvent.keyDown(input, { key: 'Enter', isComposing: true })
  expect(sends).toBe(0)
  fireEvent.keyDown(input, { key: 'Enter' })
  await waitFor(() => expect(sends).toBe(1))
})

test('首页首条会话尚未创建完成时新建对话，过期创建不能跳回或继续发送', async () => {
  const created = deferred<Response>()
  let creating = false
  let sends = 0
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (url.endsWith('/chat/sessions') && options.method === 'POST') { creating = true; return created.promise }
    if (url.endsWith('/messages') && options.method === 'POST') { sends += 1; return new Response('') }
    if (url.endsWith('/ai/settings')) return response({ ...aiSettings, chat_model_id: chatModel.id })
    return response(aiFixture(url))
  })
  boot()
  fireEvent.change(await screen.findByLabelText('输入消息'), { target: { value: '已经放弃的旧问题' } })
  fireEvent.click(screen.getByRole('button', { name: '发送' }))
  await waitFor(() => expect(creating).toBe(true))
  fireEvent.click(screen.getByRole('button', { name: '新对话' }))
  await act(async () => { created.resolve(response(session)); await created.promise })
  expect(sends).toBe(0)
  expect(screen.getByRole('heading', { name: '今天想了解什么？' })).toBeTruthy()
  expect(screen.getByLabelText('输入消息')).toHaveProperty('value', '')
  expect(screen.queryByRole('button', { name: '停止生成' })).toBeNull()
})

test('从首页选择知识库创建会话，消费流式回答与引用，刷新后读取服务端消息', async () => {
  let stored = session
  let messages: Message[] = []
  const writes: { url: string; body: unknown }[] = []
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (url.endsWith('/messages') && options.method === 'POST') {
      const user: Message = { ...assistant, id: '101', role: 'user', content: '审批人是谁？', status: 'completed' }
      const final: Message = { ...assistant, content: '审批人是林沐。[1]', status: 'completed', sources: [source] }
      messages = [user, final]
      const encode = new TextEncoder()
      return new Response(new ReadableStream({ start(controller) {
        for (const [type, data] of [['message', assistant], ['sources', [source]], ['delta', { content: final.content }], ['done', final]]) controller.enqueue(encode.encode(`event: ${String(type)}\ndata: ${JSON.stringify(data)}\n\n`))
        controller.close()
      } }), { headers: { 'Content-Type': 'text/event-stream' } })
    }
    if (url.endsWith('/chat/sessions') && options.method === 'POST') {
      const body: unknown = JSON.parse(String(options.body))
      writes.push({ url, body }); stored = { ...session, knowledge_base_ids: ['42'] }; return response(stored)
    }
    if (url.endsWith('/chat/sessions')) return response(writes.length ? [stored] : [])
    if (url.endsWith('/chat/sessions/55/messages')) return response(messages)
    if (url.endsWith('/chat/sessions/55')) return response(stored)
    if (url.endsWith('/ai/models')) return response([chatModel])
    if (url.endsWith('/ai/settings')) return response({ ...aiSettings, chat_model_id: chatModel.id })
    if (url.endsWith('/knowledge/bases')) return response([knowledgeBase])
    return response(aiFixture(url))
  })
  const view = boot()
  expect(await screen.findByRole('heading', { name: '今天想了解什么？' })).toBeTruthy()
  fireEvent.click(screen.getByRole('button', { name: '选择知识库' }))
  fireEvent.click(await screen.findByLabelText(/员工手册/))
  fireEvent.click(screen.getByRole('button', { name: '选择知识库' }))
  fireEvent.change(screen.getByLabelText('输入消息'), { target: { value: '审批人是谁？' } })
  fireEvent.click(screen.getByRole('button', { name: '发送' }))
  expect(await screen.findByText('审批人是林沐。[1]')).toBeTruthy()
  await waitFor(() => expect(screen.queryByRole('button', { name: '停止生成' })).toBeNull())
  fireEvent.click(screen.getByRole('button', { name: '参考资料 · 1' }))
  expect(await screen.findByText('1. 航星审批规则')).toBeTruthy()
  expect(screen.getByText(source.content)).toBeTruthy()
  expect(writes).toEqual([{ url: '/api/chat/sessions', body: { ...conversationDefaults({ ...aiSettings, chat_model_id: chatModel.id }), knowledge_base_ids: ['42'] } }])
  view.unmount()
  boot('/chat/55')
  expect(await screen.findByText('审批人是林沐。[1]')).toBeTruthy()
  expect(screen.queryByLabelText('对话模型')).toBeNull()
})

test('新建会话的URL延迟提交不能关闭当前会话已展开的参考资料', async () => {
  const navigation = deferred<null>()
  const final: Message = { ...assistant, content: '审批人是林沐。[1]', status: 'completed', sources: [source] }
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (url.endsWith('/messages') && options.method === 'POST') return new Response(`event: message\ndata: ${JSON.stringify(assistant)}\n\nevent: done\ndata: ${JSON.stringify(final)}\n\n`, { headers: { 'Content-Type': 'text/event-stream' } })
    if (url.endsWith('/chat/sessions/55/messages')) return response([final])
    if (url.endsWith('/chat/sessions') && options.method === 'POST') return response(session)
    if (url.endsWith('/chat/sessions')) return response([session])
    if (url.endsWith('/ai/models')) return response([chatModel])
    if (url.endsWith('/ai/settings')) return response({ ...aiSettings, chat_model_id: chatModel.id })
    return response(aiFixture(url))
  })
  const router = createMemoryRouter([
    { path: '/', element: <ChatPage api={api} /> },
    { path: '/chat/:id', loader: () => navigation.promise, element: <ChatPage api={api} /> },
  ])
  const view = render(<RouterProvider router={router} />)
  try {
    fireEvent.change(await screen.findByLabelText('输入消息'), { target: { value: '审批人是谁？' } })
    fireEvent.click(screen.getByRole('button', { name: '发送' }))
    await waitFor(() => expect(screen.queryByRole('button', { name: '停止生成' })).toBeNull())
    fireEvent.click(await screen.findByRole('button', { name: '参考资料 · 1' }))
    expect(screen.getByText('1. 航星审批规则')).toBeTruthy()
    expect(router.state.location.pathname).toBe('/')
    await act(async () => { navigation.resolve(null); await navigation.promise })
    await waitFor(() => expect(router.state.location.pathname).toBe('/chat/55'))
    expect(screen.getByText('1. 航星审批规则')).toBeTruthy()
  } finally { view.unmount(); router.dispose() }
})

test('停止生成读取服务端取消状态，对话设置保存失败仍保留可见草稿', async () => {
  let messages: Message[] = [{ ...assistant }]
  let channel: ReadableStreamDefaultController<Uint8Array> | undefined
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (url.endsWith('/stop')) { messages = [{ ...assistant, status: 'cancelled', content: '部分回答' }]; return response({ ok: true }) }
    if (url.endsWith('/messages') && options.method === 'POST') {
      return new Response(new ReadableStream({ start(controller) {
        channel = controller
        controller.enqueue(new TextEncoder().encode(`event: message\ndata: ${JSON.stringify(assistant)}\n\n`))
        options.signal?.addEventListener('abort', () => controller.error(new DOMException('Aborted', 'AbortError')), { once: true })
      } }), { headers: { 'Content-Type': 'text/event-stream' } })
    }
    if (url.endsWith('/chat/sessions/55') && options.method === 'PUT') return response(session)
    if (url.endsWith('/chat/sessions/55/messages')) return response(messages)
    if (url.endsWith('/chat/sessions/55')) return response(session)
    if (url.endsWith('/chat/sessions')) return response([session])
    if (url.endsWith('/ai/models')) return response([chatModel])
    return response(aiFixture(url))
  })
  messages = []
  boot('/chat/55')
  const input = await screen.findByLabelText('输入消息')
  await waitFor(() => expect(input).toHaveProperty('disabled', false))
  fireEvent.change(input, { target: { value: '请继续' } })
  fireEvent.click(screen.getByRole('button', { name: '发送' }))
  expect(await screen.findByRole('button', { name: '停止生成' })).toBeTruthy()
  await waitFor(() => expect(channel).toBeDefined())
  fireEvent.click(screen.getByRole('button', { name: '停止生成' }))
  expect(await screen.findByText('已停止')).toBeTruthy()
  expect(screen.getByText('部分回答')).toBeTruthy()
  fireEvent.click(screen.getByRole('button', { name: '对话操作' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: '当前对话设置' }))
  const dialog = screen.getByRole('dialog')
  fireEvent.change(within(dialog).getByLabelText('对话标题'), { target: { value: '保留草稿' } })
  vi.stubGlobal('fetch', async () => new Response(JSON.stringify({ error: { code: 'invalid_input', message: '配置暂不能保存' } }), { status: 400 }))
  fireEvent.click(within(dialog).getByRole('button', { name: '应用到当前对话' }))
  expect((await within(dialog).findByRole('alert')).textContent).toContain('配置暂不能保存')
  expect(within(dialog).getByLabelText('对话标题')).toHaveProperty('value', '保留草稿')
})

test('切换新对话取消旧连接，旧消息不能污染新页面', async () => {
  let aborted = false
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (url.endsWith('/messages') && options.method === 'POST') return new Response(new ReadableStream({ start(controller) {
      controller.enqueue(new TextEncoder().encode(`event: message\ndata: ${JSON.stringify(assistant)}\n\n`))
      options.signal?.addEventListener('abort', () => { aborted = true; controller.error(new DOMException('Aborted', 'AbortError')) }, { once: true })
    } }), { headers: { 'Content-Type': 'text/event-stream' } })
    if (url.endsWith('/chat/sessions/55')) return response(session)
    if (url.endsWith('/chat/sessions')) return response([session])
    if (url.endsWith('/ai/models')) return response([chatModel])
    return response(aiFixture(url))
  })
  boot('/chat/55')
  const input = await screen.findByLabelText('输入消息')
  await waitFor(() => expect(input).toHaveProperty('disabled', false))
  fireEvent.change(input, { target: { value: '旧问题' } })
  fireEvent.click(screen.getByRole('button', { name: '发送' }))
  expect(await screen.findByRole('button', { name: '停止生成' })).toBeTruthy()
  fireEvent.click(screen.getByRole('button', { name: '新对话' }))
  await act(async () => { await Promise.resolve() })
  await waitFor(() => expect(aborted).toBe(true))
  expect(screen.queryByText('旧问题')).toBeNull()
  expect(screen.getByRole('heading', { name: '今天想了解什么？' })).toBeTruthy()
})

test('绑定的知识库删除后显式展示，用户可取消后保存会话', async () => {
  let saved: unknown
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (options.method === 'PUT') { saved = JSON.parse(String(options.body)); return response(session) }
    if (url.endsWith('/chat/sessions/55')) return response({ ...session, knowledge_base_ids: ['404'] })
    if (url.endsWith('/chat/sessions')) return response([session])
    if (url.endsWith('/ai/models')) return response([chatModel])
    return response(aiFixture(url))
  })
  boot('/chat/55')
  await screen.findByLabelText('输入消息')
  await waitFor(() => expect(screen.getByRole('button', { name: '选择知识库' })).toHaveProperty('disabled', false))
  fireEvent.click(screen.getByRole('button', { name: '选择知识库' }))
  const unavailable = await screen.findByLabelText(/已删除的知识库（ID：404）/)
  expect(unavailable).toHaveProperty('checked', true)
  fireEvent.click(unavailable)
  expect(screen.queryByText('已删除的知识库（ID：404）')).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: '选择知识库' }))
  fireEvent.click(screen.getByRole('button', { name: '对话操作' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: '当前对话设置' }))
  fireEvent.click(screen.getByRole('button', { name: '应用到当前对话' }))
  await waitFor(() => expect(saved).toMatchObject({ knowledge_base_ids: [] }))
})

test('流结束后的权威读取完成前仍保持忙碌，不允许下一轮覆盖消息或草稿', async () => {
  const confirmed = deferred<Response>()
  let reads = 0
  let sends = 0
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (url.endsWith('/messages') && options.method === 'POST') {
      sends += 1
      return new Response(`event: message\ndata: ${JSON.stringify(assistant)}\n\nevent: done\ndata: ${JSON.stringify({ ...assistant, status: 'completed', content: '第一轮回答' })}\n\n`, { headers: { 'Content-Type': 'text/event-stream' } })
    }
    if (url.endsWith('/chat/sessions/55/messages')) { reads += 1; return reads === 1 ? response([]) : confirmed.promise }
    if (url.endsWith('/chat/sessions/55')) return response(session)
    if (url.endsWith('/chat/sessions')) return response([session])
    if (url.endsWith('/ai/models')) return response([chatModel])
    return response(aiFixture(url))
  })
  boot('/chat/55')
  const input = await screen.findByLabelText('输入消息')
  await waitFor(() => expect(input).toHaveProperty('disabled', false))
  fireEvent.change(input, { target: { value: '第一轮问题' } })
  fireEvent.click(screen.getByRole('button', { name: '发送' }))
  expect(await screen.findByText('第一轮回答')).toBeTruthy()
  expect(screen.getByLabelText('输入消息')).toHaveProperty('disabled', false)
  fireEvent.change(input, { target: { value: '生成中的第二轮草稿' } })
  fireEvent.keyDown(input, { key: 'Enter' })
  expect(screen.queryByRole('button', { name: '发送' })).toBeNull()
  expect(sends).toBe(1)
  await act(async () => { confirmed.resolve(response([{ ...assistant, status: 'completed', content: '第一轮回答' }])); await confirmed.promise })
  await waitFor(() => expect(screen.getByLabelText('输入消息')).toHaveProperty('disabled', false))
  expect(screen.getByLabelText('输入消息')).toHaveProperty('value', '生成中的第二轮草稿')
  fireEvent.change(screen.getByLabelText('输入消息'), { target: { value: '第二轮草稿' } })
  await act(async () => { await Promise.resolve() })
  expect(screen.getByLabelText('输入消息')).toHaveProperty('value', '第二轮草稿')
})

test('新会话发送被服务端明确拒绝时保留草稿，不能把创建会话当作消息提交成功', async () => {
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (url.endsWith('/messages') && options.method === 'POST') return new Response(JSON.stringify({ error: { code: 'invalid_input', message: '本次消息不能提交' } }), { status: 400 })
    if (url.endsWith('/chat/sessions') && options.method === 'POST') return response(session)
    if (url.endsWith('/chat/sessions')) return response([])
    if (url.endsWith('/chat/sessions/55/messages')) return response([])
    if (url.endsWith('/ai/models')) return response([chatModel])
    if (url.endsWith('/ai/settings')) return response({ ...aiSettings, chat_model_id: chatModel.id })
    return response(aiFixture(url))
  })
  boot()
  fireEvent.change(await screen.findByLabelText('输入消息'), { target: { value: '必须保留的原始问题' } })
  fireEvent.click(screen.getByRole('button', { name: '发送' }))
  expect((await screen.findByRole('alert')).textContent).toContain('本次消息不能提交')
  await waitFor(() => expect(screen.getByLabelText('输入消息')).toHaveProperty('disabled', false))
  expect(screen.getByLabelText('输入消息')).toHaveProperty('value', '必须保留的原始问题')
  expect(screen.queryByText('必须保留的原始问题', { selector: '.chat-message-content' })).toBeNull()
})
