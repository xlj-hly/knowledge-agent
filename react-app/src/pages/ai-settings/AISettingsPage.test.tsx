import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { MemoryRouter } from 'react-router'
import { createApi } from '../../api/client'
import { AISettingsPage } from './AISettingsPage'
import { KnowledgeForm } from '../knowledge/components/KnowledgeForm'
import { aiFixture, aiPresets, aiSettings } from '../../../tests/fixtures/ai'
import { knowledgeBase } from '../../../tests/fixtures/knowledge'

const response = (data: unknown) => new Response(JSON.stringify({ data }))
beforeEach(() => {
  // jsdom 没有布局观察与伪元素样式；不替换被测的表单和状态逻辑。
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
  const computedStyle = window.getComputedStyle.bind(window)
  vi.stubGlobal('getComputedStyle', (element: Element) => computedStyle(element))
  vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({ matches: false, addListener: vi.fn(), removeListener: vi.fn(), addEventListener: vi.fn(), removeEventListener: vi.fn(), dispatchEvent: vi.fn() }))
})
afterEach(() => { cleanup(); vi.unstubAllGlobals() })
function boot() {
  return render(<ConfigProvider virtual={false} theme={{ token: { motion: false } }}><MemoryRouter><AISettingsPage api={createApi({})} /></MemoryRouter></ConfigProvider>)
}
async function select(label: string, option: string) {
  fireEvent.mouseDown(screen.getByRole('combobox', { name: label }))
  fireEvent.click(await screen.findByRole('option', { name: option }))
}

test('默认配置只显示摘要，单项取消不保存草稿，重新编辑恢复已保存值', async () => {
  const writes: unknown[] = []
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (options.method === 'PUT') writes.push(JSON.parse(String(options.body)))
    return response(aiFixture(url))
  })
  boot()
  fireEvent.click(await screen.findByRole('button', { name: '编辑系统提示词' }))
  expect(screen.queryByRole('combobox', { name: '默认 LLM' })).toBeNull()
  expect(screen.queryByRole('spinbutton', { name: '分块长度' })).toBeNull()
  fireEvent.change(screen.getByRole('textbox', { name: '系统提示词' }), { target: { value: '不会保存的草稿' } })
  fireEvent.click(screen.getByRole('button', { name: '取消' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(writes).toEqual([])
  expect(screen.queryByRole('textbox')).toBeNull()
  expect(screen.queryByRole('navigation', { name: 'AI 配置分类' })).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: '编辑系统提示词' }))
  expect(screen.getByRole('textbox', { name: '系统提示词' })).toHaveProperty('value', aiSettings.system_prompt)
})

test('分组保存不覆盖前一项，预设与切分预览使用实际参数，修改参数后清除旧预览', async () => {
  const writes: unknown[] = []
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (url.endsWith('/preview')) return response([{ content: '审批人是林沐。', context_header: '航星计划 > 审批', index: 0, start: 12, end: 20 }])
    if (options.method === 'PUT') { const saved = JSON.parse(String(options.body)); writes.push(saved); return response({ ...aiSettings, ...saved }) }
    return response(aiFixture(url))
  })
  boot()
  fireEvent.click(await screen.findByRole('button', { name: '编辑系统提示词' }))
  await select('提示词预设', '通用助手')
  expect(screen.getByRole('textbox', { name: '系统提示词' })).toHaveProperty('value', '你是一个严谨的企业助手。')
  fireEvent.click(screen.getByRole('button', { name: '保存' }))
  expect(await screen.findByRole('status')).toHaveProperty('textContent', '系统提示词已保存')
  fireEvent.click(screen.getByRole('button', { name: '编辑文本分割与解析' }))
  await select('切分预设', '通用资料')
  expect(screen.getByRole('spinbutton', { name: '分块长度' })).toHaveProperty('value', '1024')
  fireEvent.change(screen.getByLabelText('切分预览样本'), { target: { value: '# 航星计划\n审批人是林沐。' } })
  fireEvent.click(screen.getByRole('button', { name: '预览分块' }))
  expect(await screen.findByText('航星计划 > 审批')).toBeTruthy()
  expect(screen.getByText('分块 1 · 字符 12–20')).toBeTruthy()
  fireEvent.change(screen.getByRole('spinbutton', { name: '分块长度' }), { target: { value: '512' } })
  expect(screen.queryByText('航星计划 > 审批')).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: '保存' }))
  expect(await screen.findByRole('status')).toHaveProperty('textContent', '文本分割与解析已保存')
  expect(writes).toHaveLength(2)
  expect(writes[0]).toMatchObject({ system_prompt: '你是一个严谨的企业助手。', chunk_strategy: 'auto', chunk_size: 512, chunk_overlap: 80 })
  expect(writes[1]).toMatchObject({ system_prompt: '你是一个严谨的企业助手。', chunk_strategy: 'recursive', chunk_size: 512, chunk_overlap: 100 })
  for (const input of writes) expect(input).not.toHaveProperty('chat_model_id')
})

test('保存期间禁止取消，失败保留当前草稿与明确错误，重试可以保存', async () => {
  let settle: ((response: Response) => void) | undefined
  let attempts = 0
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (options.method === 'PUT') {
      attempts += 1
      if (attempts === 1) return new Promise<Response>(resolve => { settle = resolve })
      return response({ ...aiSettings, ...JSON.parse(String(options.body)) })
    }
    return response(aiFixture(url))
  })
  boot()
  fireEvent.click(await screen.findByRole('button', { name: '编辑系统提示词' }))
  fireEvent.change(screen.getByRole('textbox', { name: '系统提示词' }), { target: { value: '请用资料回答' } })
  fireEvent.click(screen.getByRole('button', { name: '保存' }))
  expect(screen.getByRole('button', { name: '取消' })).toHaveProperty('disabled', true)
  settle?.(new Response(JSON.stringify({ error: { code: 'conflict', message: '配置已被修改，请重试' } }), { status: 409 }))
  expect(await screen.findByRole('alert')).toHaveProperty('textContent', '配置已被修改，请重试')
  expect(screen.getByRole('textbox', { name: '系统提示词' })).toHaveProperty('value', '请用资料回答')
  await waitFor(() => expect(screen.getByRole('button', { name: '保存' }).classList.contains('ant-btn-loading')).toBe(false))
  fireEvent.click(screen.getByRole('button', { name: '保存' }))
  expect(await screen.findByRole('status')).toHaveProperty('textContent', '系统提示词已保存')
  expect(screen.getByText('请用资料回答')).toBeTruthy()
})

test('知识库默认连接不再提供 LLM 选择，保存模板不提交旧启用模型', async () => {
  let body: unknown
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (options.method === 'PUT') {
      body = JSON.parse(String(options.body))
      return response({ ...aiSettings, ...JSON.parse(String(options.body)), chat_model_id: '93' })
    }
    return response(url.endsWith('/ai/settings') ? { ...aiSettings, chat_model_id: '92' } : aiFixture(url))
  })
  boot()
  fireEvent.click(await screen.findByRole('button', { name: '编辑知识库默认连接' }))
  expect(screen.queryByRole('combobox', { name: '默认 LLM' })).toBeNull()
  expect(screen.getByRole('combobox', { name: '默认 Embedder' })).toBeTruthy()
  expect(screen.getByRole('combobox', { name: '默认向量存储' })).toBeTruthy()
  fireEvent.click(screen.getByRole('button', { name: '保存' }))
  expect(await screen.findByRole('status')).toHaveProperty('textContent', '知识库默认连接已保存')
  expect(body).not.toHaveProperty('chat_model_id')
  expect(body).toMatchObject({ embedding_model_id: '', vector_store_id: '' })
})

test('新建知识库继承当前默认配置，已有知识库仍使用创建时的配置', async () => {
  Object.defineProperty(HTMLDialogElement.prototype, 'showModal', { configurable: true, value() { this.setAttribute('open', '') } })
  Object.defineProperty(HTMLDialogElement.prototype, 'close', { configurable: true, value() { this.removeAttribute('open') } })
  vi.stubGlobal('fetch', async (url: string) => response(url.endsWith('/ai/settings') ? { ...aiSettings, chunk_strategy: 'heading', chunk_size: 1536, chunk_overlap: 200 } : aiFixture(url)))
  const api = createApi({})
  const view = render(<MemoryRouter><KnowledgeForm api={api} onClose={vi.fn()} onSaved={vi.fn()} /></MemoryRouter>)
  expect(await screen.findByLabelText('分块长度', { exact: false })).toHaveProperty('value', '1536')
  expect(screen.getByLabelText('切分策略')).toHaveProperty('value', 'heading')
  view.unmount()
  render(<MemoryRouter><KnowledgeForm api={api} base={knowledgeBase} onClose={vi.fn()} onSaved={vi.fn()} /></MemoryRouter>)
  await waitFor(() => expect(screen.getByLabelText('分块长度', { exact: false })).toHaveProperty('value', '512'))
  expect(screen.getByLabelText('切分策略')).toHaveProperty('value', 'auto')
})

test('默认配置摘要和编辑器读取服务端新增的切分策略名称与说明', async () => {
  vi.stubGlobal('fetch', async (url: string) => response(url.endsWith('/ai/presets')
    ? { ...aiPresets, chunk_strategies: [{ id: 'catalog-test', name: '目录中的新策略', description: '服务端提供的策略说明。' }] }
    : url.endsWith('/ai/settings') ? { ...aiSettings, chunk_strategy: 'catalog-test' } : aiFixture(url)))
  boot()
  expect(await screen.findByText('目录中的新策略 · 每块 512 字 · 重叠 80 字')).toBeTruthy()
  fireEvent.click(screen.getByRole('button', { name: '编辑文本分割与解析' }))
  fireEvent.mouseDown(screen.getByRole('combobox', { name: '切分策略' }))
  expect(await screen.findByRole('option', { name: '目录中的新策略' })).toBeTruthy()
  expect(screen.getByText('服务端提供的策略说明。')).toBeTruthy()
})
