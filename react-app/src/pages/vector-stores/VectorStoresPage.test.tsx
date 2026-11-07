import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { createApi } from '../../api/client'
import { VectorStoresPage } from './VectorStoresPage'
import type { VectorStore } from '../../api/knowledge/types'
import { vectorProviders } from '../../../tests/fixtures/knowledge'

const response = (data: unknown) => new Response(JSON.stringify({ data }))
const connection: VectorStore = { id: '71', name: '企业索引', provider: 'qdrant', config: { endpoint: 'http://qdrant:6333', collection: 'agent', api_key: 'fixture-vector-key' }, referenced: false, created_at: '', updated_at: '' }
afterEach(() => { cleanup(); vi.unstubAllGlobals() })

test('引擎卡直达模块字段表单，不重复提供预设下拉', async () => {
  let saved: unknown
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (url.endsWith('/vector-providers')) return response(vectorProviders)
    if (options.method === 'POST') { saved = JSON.parse(String(options.body)); return response(connection) }
    return response([])
  })
  render(<VectorStoresPage api={createApi({})} />)
  await screen.findByText('尚未配置向量存储')
  expect(screen.queryByLabelText('引擎预设')).toBeNull()
  expect(screen.queryByText('Milvus')).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: '添加 Qdrant 连接' }))
  expect(screen.getAllByRole('dialog')).toHaveLength(1)
  expect(screen.getByLabelText('服务地址')).toHaveProperty('value', 'http://host.docker.internal:6333')
  fireEvent.change(screen.getByLabelText('连接名称'), { target: { value: '企业索引' } })
  fireEvent.click(screen.getByRole('button', { name: '保存连接' }))
  expect(await screen.findByText('向量存储配置已保存。')).toBeTruthy()
  expect(saved).toEqual({ name: '企业索引', provider: 'qdrant', config: { endpoint: 'http://host.docker.internal:6333', collection: 'agent_knowledge', api_key: '' } })
})

test('编辑连接回填密钥，取消丢弃草稿，清空密钥保存失败仍保留表单', async () => {
  let saved: unknown
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (url.endsWith('/vector-providers')) return response(vectorProviders)
    if (options.method === 'PATCH') {
      saved = JSON.parse(String(options.body))
      return new Response(JSON.stringify({ error: { code: 'conflict', message: '连接正在使用' } }), { status: 409 })
    }
    return response([connection])
  })
  render(<VectorStoresPage api={createApi({})} />)
  fireEvent.click(await screen.findByRole('button', { name: '编辑' }))
  expect(screen.getByLabelText('API Key')).toHaveProperty('value', connection.config.api_key)
  fireEvent.change(screen.getByLabelText('连接名称'), { target: { value: '未保存' } })
  fireEvent.change(screen.getByLabelText('API Key'), { target: { value: 'cancelled-key' } })
  fireEvent.click(screen.getByRole('button', { name: '取消' }))
  fireEvent.click(screen.getByRole('button', { name: '编辑' }))
  expect(screen.getByLabelText('连接名称')).toHaveProperty('value', '企业索引')
  expect(screen.getByLabelText('API Key')).toHaveProperty('value', connection.config.api_key)
  fireEvent.change(screen.getByLabelText('API Key'), { target: { value: '' } })
  fireEvent.click(screen.getByRole('button', { name: '保存连接' }))
  await waitFor(() => expect(screen.getByRole('alert').textContent).toContain('连接正在使用'))
  expect(saved).toMatchObject({ config: { api_key: '' } })
  expect(screen.getByLabelText('连接名称')).toHaveProperty('value', '企业索引')
  expect(screen.getByLabelText('API Key')).toHaveProperty('value', '')
})

test.each([{ restore: false, label: '修改地址保留显式密钥草稿' }, { restore: true, label: '改回原地址也提交当前密钥草稿' }])('$label', async ({ restore }) => {
  let saved: Record<string, unknown> | undefined
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (url.endsWith('/vector-providers')) return response(vectorProviders)
    if (options.method === 'PATCH') { saved = JSON.parse(String(options.body)); return response(connection) }
    return response([connection])
  })
  render(<VectorStoresPage api={createApi({})} />)
  fireEvent.click(await screen.findByRole('button', { name: '编辑' }))
  fireEvent.change(screen.getByLabelText('API Key'), { target: { value: 'draft-secret' } })
  fireEvent.change(screen.getByLabelText('服务地址'), { target: { value: 'https://new-qdrant.example:6333' } })
  expect(screen.getByLabelText('API Key')).toHaveProperty('value', 'draft-secret')
  if (restore) fireEvent.change(screen.getByLabelText('服务地址'), { target: { value: connection.config.endpoint + '/' } })
  fireEvent.click(screen.getByRole('button', { name: '保存连接' }))
  await screen.findByText('向量存储配置已保存。')
  expect(saved).toMatchObject({ config: { api_key: 'draft-secret', endpoint: restore ? connection.config.endpoint + '/' : 'https://new-qdrant.example:6333' } })
})

test.each([
  { field: 'username', label: '用户名', previous: 'user/', next: 'user' },
  { field: 'username', label: '用户名', previous: 'user', next: 'user/' },
  { field: 'username', label: '用户名', previous: 'user', next: ' user ' },
  { field: 'endpoint', label: '服务地址', previous: connection.config.endpoint, next: ` ${connection.config.endpoint}/ ` },
])('修改普通配置 $field 时不自动清空已回填凭据', async ({ field, label, previous, next }) => {
  const provider = { ...vectorProviders[0], id: 'account-vector', name: 'Account Vector', fields: [...vectorProviders[0].fields, { ...vectorProviders[0].fields[2], key: 'username', label: '用户名', default: '' }] }
  const store = { ...connection, provider: provider.id, config: { ...connection.config, username: 'user', [field]: previous } }
  let saved: Record<string, unknown> | undefined
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (url.endsWith('/vector-providers')) return response([provider])
    if (options.method === 'PATCH') { saved = JSON.parse(String(options.body)); return response(store) }
    return response([store])
  })
  render(<VectorStoresPage api={createApi({})} />)
  fireEvent.click(await screen.findByRole('button', { name: '编辑' }))
  fireEvent.change(screen.getByLabelText(label), { target: { value: next } })
  expect(screen.getByLabelText('API Key')).toHaveProperty('value', connection.config.api_key)
  fireEvent.click(screen.getByRole('button', { name: '保存连接' }))
  await screen.findByText('向量存储配置已保存。')
  expect(saved).toMatchObject({ config: { [field]: next.trim(), api_key: connection.config.api_key } })
})

test('向量凭据改值和清空后重新编辑读取保存值，列表依据实际配置显示状态', async () => {
  let saved = { ...connection, config: { ...connection.config } }
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (url.endsWith('/vector-providers')) return response(vectorProviders)
    if (options.method === 'PATCH') { saved = { ...saved, ...JSON.parse(String(options.body)) }; return response(saved) }
    return response([saved])
  })
  render(<VectorStoresPage api={createApi({})} />)
  fireEvent.click(await screen.findByRole('button', { name: '编辑' }))
  fireEvent.change(screen.getByLabelText('API Key'), { target: { value: 'updated-vector-key' } })
  fireEvent.click(screen.getByRole('button', { name: '保存连接' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  fireEvent.click(await screen.findByRole('button', { name: '编辑' }))
  expect(screen.getByLabelText('API Key')).toHaveProperty('value', 'updated-vector-key')
  fireEvent.change(screen.getByLabelText('API Key'), { target: { value: '' } })
  fireEvent.click(screen.getByRole('button', { name: '保存连接' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  await screen.findByText('未配置')
  fireEvent.click(await screen.findByRole('button', { name: '编辑' }))
  expect(screen.getByLabelText('API Key')).toHaveProperty('value', '')
})

test('向量引擎的品牌、默认连接与保存类型全部来自服务端注册目录', async () => {
  const provider = { ...vectorProviders[0], id: 'new-vector', name: 'New Vector', description: '新注册的向量服务', fields: vectorProviders[0].fields.map(field => ({ ...field, default: field.key === 'endpoint' ? 'http://new-vector.test:9000' : field.key === 'collection' ? 'new_collection' : field.default })) }
  let saved: unknown
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (url.endsWith('/vector-providers')) return response([provider])
    if (options.method === 'POST') { saved = JSON.parse(String(options.body)); return response({ ...connection, provider: provider.id }) }
    return response([])
  })
  render(<VectorStoresPage api={createApi({})} />)
  expect(await screen.findByRole('heading', { name: 'New Vector' })).toBeTruthy()
  expect(screen.queryByText('Qdrant')).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: '添加 New Vector 连接' }))
  expect(screen.getByLabelText('服务地址')).toHaveProperty('value', 'http://new-vector.test:9000')
  expect(screen.getByLabelText('集合名称')).toHaveProperty('value', 'new_collection')
  fireEvent.change(screen.getByLabelText('连接名称'), { target: { value: '新引擎连接' } })
  fireEvent.click(screen.getByRole('button', { name: '保存连接' }))
  await screen.findByText('向量存储配置已保存。')
  expect(saved).toEqual({ name: '新引擎连接', provider: provider.id, config: { endpoint: 'http://new-vector.test:9000', collection: 'new_collection', api_key: '' } })
})

test('使用中的连接禁改归属字段，仍可更新凭据；引擎可搜索', async () => {
  vi.stubGlobal('fetch', async (url: string) => response(url.endsWith('/vector-providers') ? vectorProviders : [{ ...connection, referenced: true }]))
  render(<VectorStoresPage api={createApi({})} />)
  fireEvent.click(await screen.findByRole('button', { name: '编辑' }))
  expect(screen.getByLabelText('服务地址')).toHaveProperty('disabled', true)
  expect(screen.getByLabelText('集合名称')).toHaveProperty('disabled', true)
  expect(screen.getByLabelText('API Key')).toHaveProperty('disabled', false)
  fireEvent.click(screen.getByRole('button', { name: '取消' }))
  fireEvent.change(screen.getByLabelText('搜索向量引擎'), { target: { value: '没有这个引擎' } })
  expect(screen.getByText('没有匹配的向量引擎')).toBeTruthy()
  expect(screen.getByRole('button', { name: '删除' })).toHaveProperty('disabled', true)
})

test('新增模块的数字、选项与布尔字段直接呈现并保存，不依赖品牌分支', async () => {
  const base = vectorProviders[0].fields[0]
  const provider = { ...vectorProviders[0], id: 'future', name: 'Future', fields: [
    { ...base, key: 'port', label: '端口', type: 'number', default: '5432' },
    { ...base, key: 'tls', label: '启用 TLS', type: 'boolean', default: 'false' },
    { ...base, key: 'metric', label: '距离类型', type: 'select', default: 'cosine', options: [{ value: 'cosine', label: '余弦' }, { value: 'dot', label: '点积' }] },
  ] }
  let saved: unknown
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (url.endsWith('/vector-providers')) return response([provider])
    if (options.method === 'POST') { saved = JSON.parse(String(options.body)); return response({ ...connection, provider: 'future' }) }
    return response([])
  })
  render(<VectorStoresPage api={createApi({})} />)
  fireEvent.click(await screen.findByRole('button', { name: '添加 Future 连接' }))
  fireEvent.change(screen.getByLabelText('连接名称'), { target: { value: '扩展连接' } })
  fireEvent.change(screen.getByLabelText('端口'), { target: { value: '6432' } })
  fireEvent.click(screen.getByLabelText('启用 TLS'))
  fireEvent.change(screen.getByLabelText('距离类型'), { target: { value: 'dot' } })
  fireEvent.click(screen.getByRole('button', { name: '保存连接' }))
  await screen.findByText('向量存储配置已保存。')
  expect(saved).toEqual({ name: '扩展连接', provider: 'future', config: { port: '6432', tls: 'true', metric: 'dot' } })
})
