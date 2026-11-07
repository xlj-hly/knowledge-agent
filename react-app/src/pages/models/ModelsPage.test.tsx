import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { MemoryRouter } from 'react-router'
import { createApi } from '../../api/client'
import { ModelsPage } from './ModelsPage'
import { embeddingModel } from '../../../tests/fixtures/knowledge'
import { aiFixture, aiProviders, aiSettings, chatModel } from '../../../tests/fixtures/ai'

const response = (data: unknown) => new Response(JSON.stringify({ data }))
beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
  Object.defineProperty(HTMLDialogElement.prototype, 'showModal', { configurable: true, value() { this.setAttribute('open', '') } })
  Object.defineProperty(HTMLDialogElement.prototype, 'close', { configurable: true, value() { this.removeAttribute('open') } })
})
afterEach(() => { cleanup(); vi.unstubAllGlobals(); sessionStorage.clear(); localStorage.clear() })
function boot() { render(<MemoryRouter><ModelsPage api={createApi({})} kind="embedding" /></MemoryRouter>) }

test('模型列表启用一个 LLM，读回后唯一启用状态和重新挂载保持一致', async () => {
  const second = { ...chatModel, id: '93', name: '新的对话模型' }
  let activeID = chatModel.id
  const writes: { url: string; body: unknown }[] = []
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (url.endsWith('/activate')) {
      writes.push({ url, body: options.body })
      activeID = second.id
      return response({ ...aiSettings, chat_model_id: activeID })
    }
    return response(url.endsWith('/ai/models') ? [chatModel, second] : url.endsWith('/ai/settings') ? { ...aiSettings, chat_model_id: activeID } : aiFixture(url))
  })
  const view = render(<MemoryRouter><ModelsPage api={createApi({})} /></MemoryRouter>)
  const initial = await screen.findByText('已启用')
  expect(initial.closest('tr')?.textContent).toContain(chatModel.name)
  fireEvent.click(screen.getByRole('button', { name: '启用' }))
  expect(await screen.findByRole('status')).toHaveProperty('textContent', '已启用「新的对话模型」，后续消息将使用此模型。')
  await waitFor(() => expect(screen.getByText('已启用').closest('tr')?.textContent).toContain(second.name))
  expect(screen.getAllByText('已启用')).toHaveLength(1)
  expect(writes).toEqual([{ url: `/api/ai/models/${second.id}/activate`, body: undefined }])
  view.unmount()
  render(<MemoryRouter><ModelsPage api={createApi({})} /></MemoryRouter>)
  expect((await screen.findByText('已启用')).closest('tr')?.textContent).toContain(second.name)
})

test('启用请求等待时不允许重复切换，失败保留原启用状态并可重试', async () => {
  let settle: ((response: Response) => void) | undefined
  vi.stubGlobal('fetch', async (url: string) => {
    if (url.endsWith('/activate')) return new Promise<Response>(resolve => { settle = resolve })
    return response(url.endsWith('/ai/models') ? [chatModel, { ...chatModel, id: '93', name: '备用模型' }] : url.endsWith('/ai/settings') ? { ...aiSettings, chat_model_id: chatModel.id } : aiFixture(url))
  })
  render(<MemoryRouter><ModelsPage api={createApi({})} /></MemoryRouter>)
  fireEvent.click(await screen.findByRole('button', { name: '启用' }))
  expect(screen.getByRole('button', { name: '启用' })).toHaveProperty('disabled', true)
  settle?.(new Response(JSON.stringify({ error: { code: 'conflict', message: '模型配置不可用' } }), { status: 409 }))
  expect(await screen.findByRole('alert')).toHaveProperty('textContent', '模型配置不可用')
  expect(screen.getByText('已启用').closest('tr')?.textContent).toContain(chatModel.name)
  await waitFor(() => expect(screen.getByRole('button', { name: '启用' })).toHaveProperty('disabled', false))
})

test('嵌入器列表不提供 LLM 启用操作', async () => {
  vi.stubGlobal('fetch', async (url: string) => response(url.endsWith('/ai/models') ? [embeddingModel] : aiFixture(url)))
  boot()
  await screen.findByRole('button', { name: '编辑与测试' })
  expect(screen.queryByRole('button', { name: '启用' })).toBeNull()
})

test('已配置模型回填原密钥，实际测试与保存提交完整字段且不发送探测 ID', async () => {
  const writes: unknown[] = []
  let probe: unknown
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (url.endsWith('/test')) { probe = JSON.parse(String(options.body)); return response({ ok: true, dimension: 1024 }) }
    if (options.method === 'PUT') { writes.push(JSON.parse(String(options.body))); return response(embeddingModel) }
    return response(url.endsWith('/ai/models') ? [embeddingModel] : aiFixture(url))
  })
  boot()
  fireEvent.click(await screen.findByRole('button', { name: '编辑与测试' }))
  expect(screen.getByLabelText(/^API Key/)).toHaveProperty('value', embeddingModel.api_key)
  fireEvent.click(screen.getByRole('button', { name: '测试调用并检测维度' }))
  expect(await screen.findByText('实际调用成功，向量维度为 1024。')).toBeTruthy()
  await waitFor(() => expect(screen.getByRole('button', { name: '保存模型' })).toHaveProperty('disabled', false))
  fireEvent.click(screen.getByRole('button', { name: '保存模型' }))
  expect(await screen.findByText('模型配置已保存。')).toBeTruthy()
  expect(writes).toEqual([{ name: embeddingModel.name, kind: 'embedding', provider: 'openai', endpoint: embeddingModel.endpoint, model_name: embeddingModel.model_name, dimension: 1024, api_key: embeddingModel.api_key }])
  expect(probe).toEqual(writes[0])
  expect(sessionStorage.length).toBe(0)
  expect(localStorage.length).toBe(0)
})

test('清空输入直接提交空值，保存失败保留草稿并显示原因', async () => {
  let body: unknown
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (options.method === 'PUT') {
      body = JSON.parse(String(options.body))
      return new Response(JSON.stringify({ error: { code: 'invalid_input', message: '模型已引用，不能修改服务地址' } }), { status: 400 })
    }
    return response(url.endsWith('/ai/models') ? [embeddingModel] : aiFixture(url))
  })
  boot()
  fireEvent.click(await screen.findByRole('button', { name: '编辑与测试' }))
  fireEvent.change(screen.getByLabelText('API Key'), { target: { value: '' } })
  fireEvent.change(screen.getByLabelText('配置名称'), { target: { value: '待保存名称' } })
  fireEvent.click(screen.getByRole('button', { name: '保存模型' }))
  await waitFor(() => expect(screen.getByRole('alert').textContent).toContain('模型已引用'))
  expect(body).toMatchObject({ api_key: '' })
  expect(screen.getByLabelText('配置名称')).toHaveProperty('value', '待保存名称')
  expect(screen.getByLabelText('API Key')).toHaveProperty('value', '')
})

test('更新与清空模型密钥后重新编辑读取保存值，列表状态跟随实际值', async () => {
  let saved = { ...embeddingModel }
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (options.method === 'PUT') { saved = { ...saved, ...JSON.parse(String(options.body)) }; return response(saved) }
    return response(url.endsWith('/ai/models') ? [saved] : aiFixture(url))
  })
  boot()
  fireEvent.click(await screen.findByRole('button', { name: '编辑与测试' }))
  fireEvent.change(screen.getByLabelText('API Key'), { target: { value: 'replacement-key' } })
  fireEvent.click(screen.getByRole('button', { name: '保存模型' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  fireEvent.click(await screen.findByRole('button', { name: '编辑与测试' }))
  expect(screen.getByLabelText('API Key')).toHaveProperty('value', 'replacement-key')
  fireEvent.change(screen.getByLabelText('API Key'), { target: { value: '' } })
  fireEvent.click(screen.getByRole('button', { name: '保存模型' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  await screen.findByText('未配置')
  fireEvent.click(await screen.findByRole('button', { name: '编辑与测试' }))
  expect(screen.getByLabelText('API Key')).toHaveProperty('value', '')
})

test('取消模型编辑丢弃名称与密钥草稿，重新打开恢复已保存字段', async () => {
  vi.stubGlobal('fetch', async (url: string) => response(url.endsWith('/ai/models') ? [embeddingModel] : aiFixture(url)))
  boot()
  fireEvent.click(await screen.findByRole('button', { name: '编辑与测试' }))
  fireEvent.change(screen.getByLabelText('API Key'), { target: { value: 'cancelled-key' } })
  fireEvent.change(screen.getByLabelText('配置名称'), { target: { value: '取消的草稿' } })
  fireEvent.click(screen.getByRole('button', { name: '取消' }))
  fireEvent.click(screen.getByRole('button', { name: '编辑与测试' }))
  expect(screen.getByLabelText('API Key')).toHaveProperty('value', embeddingModel.api_key)
  expect(screen.getByLabelText('配置名称')).toHaveProperty('value', embeddingModel.name)
})

test('新增模型带入服务商地址与模型预设，并可发现服务端模型', async () => {
  vi.stubGlobal('fetch', async (url: string) => response(url.endsWith('/discover') ? { models: [{ id: 'local-embedding', name: 'Local', kind: 'embedding', dimension: 768 }] } : aiFixture(url)))
  boot()
  fireEvent.click(await screen.findByRole('button', { name: '添加模型' }))
  expect(await screen.findByLabelText('服务地址', { exact: false })).toHaveProperty('value', 'https://api.openai.com/v1')
  fireEvent.change(screen.getByLabelText('模型名称'), { target: { value: 'text-embedding-3-small' } })
  expect(screen.getByLabelText('向量维度', { exact: false })).toHaveProperty('value', '1536')
  fireEvent.click(screen.getByRole('button', { name: '获取模型列表' }))
  expect(await screen.findByText('获取到 1 个模型，可在下方选择或手动输入。')).toBeTruthy()
  fireEvent.change(screen.getByLabelText('模型名称'), { target: { value: 'local-embedding' } })
  expect(screen.getByLabelText('向量维度', { exact: false })).toHaveProperty('value', '768')
})

test('点击服务商卡片直接配置该服务商，无重复导航或第二个弹窗', async () => {
  const providers = [...aiProviders, { ...aiProviders[0], id: 'ollama', name: 'Ollama', protocol: 'ollama', endpoint: 'http://localhost:11434', kinds: ['embedding'], models: [] }]
  vi.stubGlobal('fetch', async (url: string) => response(url.endsWith('/providers') ? providers : aiFixture(url)))
  boot()
  fireEvent.click(await screen.findByRole('button', { name: '配置 Ollama' }))
  expect(screen.getByLabelText('服务地址')).toHaveProperty('value', 'http://localhost:11434')
  expect(screen.getAllByRole('dialog')).toHaveLength(1)
  expect(screen.queryByRole('navigation', { name: 'AI 配置分类' })).toBeNull()
  fireEvent.change(screen.getByLabelText('配置名称'), { target: { value: '未保存草稿' } })
  fireEvent.click(screen.getByRole('button', { name: '取消' }))
  fireEvent.click(screen.getByRole('button', { name: '配置 OpenAI' }))
  expect(screen.getByLabelText('配置名称')).toHaveProperty('value', '')
  expect(screen.getByLabelText('服务地址')).toHaveProperty('value', 'https://api.openai.com/v1')
})

test('切换服务商保留显式密钥草稿，重置模型与发现结果', async () => {
  const providers = [...aiProviders, { ...aiProviders[0], id: 'ollama', name: 'Ollama', protocol: 'ollama', endpoint: 'http://localhost:11434', kinds: ['embedding'], models: [] }]
  const probes: Record<string, unknown>[] = []
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (url.endsWith('/discover')) {
      probes.push(JSON.parse(String(options.body)))
      return response({ models: [{ id: 'old-only-model', name: 'Old', kind: 'embedding', dimension: 768 }] })
    }
    return response(url.endsWith('/providers') ? providers : url.endsWith('/ai/models') ? [embeddingModel] : aiFixture(url))
  })
  boot()
  fireEvent.click(await screen.findByRole('button', { name: '编辑与测试' }))
  fireEvent.change(screen.getByLabelText('API Key'), { target: { value: 'draft-secret' } })
  fireEvent.click(screen.getByRole('button', { name: '获取模型列表' }))
  expect(await screen.findByText('获取到 1 个模型，可在下方选择或手动输入。')).toBeTruthy()
  await waitFor(() => expect(screen.getByRole('combobox', { name: '服务商' }).hasAttribute('disabled')).toBe(false))
  fireEvent.mouseDown(screen.getByRole('combobox', { name: '服务商' }))
  fireEvent.click(await screen.findByRole('option', { name: 'Ollama' }))
  expect(screen.getByLabelText('API Key')).toHaveProperty('value', 'draft-secret')
  expect(screen.getByLabelText('模型名称')).toHaveProperty('value', '')
  expect(screen.getByLabelText('向量维度')).toHaveProperty('value', '0')
  fireEvent.click(screen.getByRole('button', { name: '获取模型列表' }))
  await waitFor(() => expect(probes).toHaveLength(2))
  expect(probes[1]).toMatchObject({ provider: 'ollama', endpoint: 'http://localhost:11434', api_key: 'draft-secret' })
  expect(probes[1]).not.toHaveProperty('id')
})

test('修改地址保留表单密钥，探测只发送当前完整表单，手动清空才提交空值', async () => {
  const probes: Record<string, unknown>[] = []
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (url.endsWith('/discover')) { probes.push(JSON.parse(String(options.body))); return response({ models: [] }) }
    return response(url.endsWith('/ai/models') ? [embeddingModel] : aiFixture(url))
  })
  boot()
  fireEvent.click(await screen.findByRole('button', { name: '编辑与测试' }))
  fireEvent.change(screen.getByLabelText('API Key'), { target: { value: 'draft-secret' } })
  fireEvent.change(screen.getByLabelText('服务地址'), { target: { value: 'https://new-model.example/v1' } })
  expect(screen.getByLabelText('API Key')).toHaveProperty('value', 'draft-secret')
  fireEvent.click(screen.getByRole('button', { name: '获取模型列表' }))
  await waitFor(() => expect(probes).toHaveLength(1))
  expect(probes[0]).toMatchObject({ endpoint: 'https://new-model.example/v1', api_key: 'draft-secret' })
  expect(probes[0]).not.toHaveProperty('id')
  await waitFor(() => expect(screen.getByLabelText('服务地址')).toHaveProperty('disabled', false))
  fireEvent.change(screen.getByLabelText('服务地址'), { target: { value: embeddingModel.endpoint + '/' } })
  fireEvent.change(screen.getByLabelText('API Key'), { target: { value: '' } })
  fireEvent.click(screen.getByRole('button', { name: '获取模型列表' }))
  await waitFor(() => expect(probes).toHaveLength(2))
  expect(probes[1]).toMatchObject({ endpoint: embeddingModel.endpoint + '/', api_key: '' })
  expect(probes[1]).not.toHaveProperty('id')
})

test('服务端新增兼容服务商可按描述搜索、显示其Logo并配置，不依赖前端品牌登记', async () => {
  const provider = { ...aiProviders[0], id: 'new-gateway', name: 'New Gateway', description: '团队自建推理网关', endpoint: 'http://gateway.test/v1', credential_required: false, discovery: false, models: [] }
  vi.stubGlobal('fetch', async (url: string) => response(url.endsWith('/providers') ? [...aiProviders, provider] : aiFixture(url)))
  boot()
  await screen.findByRole('button', { name: '配置 New Gateway' })
  fireEvent.change(screen.getByRole('textbox', { name: '搜索服务商' }), { target: { value: '自建推理' } })
  expect(screen.queryByRole('button', { name: '配置 OpenAI' })).toBeNull()
  const card = screen.getByRole('button', { name: '配置 New Gateway' })
  expect(card.querySelector('img')).toHaveProperty('src', provider.logo)
  fireEvent.click(card)
  expect(screen.getByLabelText('服务地址')).toHaveProperty('value', provider.endpoint)
  expect(screen.queryByRole('button', { name: '获取模型列表' })).toBeNull()
  expect(screen.getByText('此服务商不提供模型列表，请选择预设或手动填写模型名称。')).toBeTruthy()
  expect(screen.getByPlaceholderText('服务无需密钥时可留空')).toBeTruthy()
  fireEvent.change(screen.getByLabelText('模型名称'), { target: { value: 'custom-embedding' } })
  expect(screen.getByLabelText('模型名称')).toHaveProperty('value', 'custom-embedding')
})
