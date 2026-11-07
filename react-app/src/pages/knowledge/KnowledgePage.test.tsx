import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { MemoryRouter } from 'react-router'
import { WorkspaceRoutes } from '../../router/WorkspaceRoutes'
import { createApi } from '../../api/client'
import { alice, bob, tenant } from '../../../tests/fixtures/identity'
import { knowledgeBase } from '../../../tests/fixtures/knowledge'
import { aiFixture, aiPresets } from '../../../tests/fixtures/ai'

afterEach(() => { cleanup(); vi.unstubAllGlobals() })
beforeEach(() => {
  Object.defineProperty(HTMLDialogElement.prototype, 'showModal', { configurable: true, value() { this.setAttribute('open', '') } })
  Object.defineProperty(HTMLDialogElement.prototype, 'close', { configurable: true, value() { this.removeAttribute('open') } })
})

function boot(path = '/knowledge', admin = true) {
  vi.stubGlobal('fetch', async (url: string) => new Response(JSON.stringify({ data: aiFixture(url) })))
  render(<MemoryRouter initialEntries={[path]}><WorkspaceRoutes api={createApi({})} platform={false}
    identity={{ user: admin ? alice : bob, tenant }} onSessionEnd={() => undefined} /></MemoryRouter>)
}

test('企业成员可进入知识库，管理员可从导航配置模型和向量存储', async () => {
  boot()
  expect(screen.getByRole('link', { name: '知识库' })).toBeTruthy()
  expect(screen.getByRole('link', { name: 'LLM 对话模型' })).toBeTruthy()
  expect(screen.getByRole('link', { name: '嵌入器 Embedder' })).toBeTruthy()
  expect(screen.getByRole('link', { name: '向量存储' })).toBeTruthy()
  expect(await screen.findByRole('heading', { name: '还没有知识库' })).toBeTruthy()
})

test('向量知识库必须选择模型和连接，关键词知识库无需外部凭据即可创建', async () => {
  boot()
  fireEvent.click(await screen.findByRole('button', { name: '新建知识库' }))
  fireEvent.change(await screen.findByLabelText('知识库名称'), { target: { value: '企业资料' } })
  fireEvent.click(screen.getByRole('button', { name: '创建知识库' }))
  expect((await screen.findByRole('alert')).textContent).toContain('配置默认 Embedder 和向量存储')
  fireEvent.change(screen.getByLabelText('索引能力', { exact: false }), { target: { value: 'keyword' } })
  expect(screen.queryByLabelText('Embedding 模型')).toBeNull()
  vi.stubGlobal('fetch', async (url: string) => new Response(JSON.stringify({ data: url.endsWith('/documents') ? [] : { ...knowledgeBase, name: '企业资料' } })))
  fireEvent.click(screen.getByRole('button', { name: '创建知识库' }))
  expect(await screen.findByRole('heading', { name: '企业资料' })).toBeTruthy()
})

test('普通成员没有配置入口，直接访问配置页面会显示无权访问', async () => {
  boot('/models', false)
  expect(await screen.findByRole('heading', { name: '无权访问' })).toBeTruthy()
  expect(screen.queryByRole('link', { name: 'LLM 对话模型' })).toBeNull()
  expect(screen.getByRole('link', { name: '知识库' })).toBeTruthy()
})

test('知识库表单读取服务端新增的切分策略目录，仅展示与选择元数据', async () => {
  boot()
  vi.stubGlobal('fetch', async (url: string) => new Response(JSON.stringify({ data: url.endsWith('/ai/presets')
    ? { ...aiPresets, chunk_strategies: [{ id: 'catalog-test', name: '目录中的新策略', description: '服务端提供的策略说明。' }] }
    : aiFixture(url) })))
  fireEvent.click(await screen.findByRole('button', { name: '新建知识库' }))
  const strategy = await screen.findByRole('combobox', { name: '切分策略' })
  expect(screen.getByRole('option', { name: '目录中的新策略' })).toHaveProperty('value', 'catalog-test')
  fireEvent.change(strategy, { target: { value: 'catalog-test' } })
  expect(strategy).toHaveProperty('value', 'catalog-test')
  expect(screen.getByText('服务端提供的策略说明。')).toBeTruthy()
})
