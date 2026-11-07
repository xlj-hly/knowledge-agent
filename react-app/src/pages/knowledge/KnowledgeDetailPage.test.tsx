import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router'
import { createApi } from '../../api/client'
import { KnowledgeDetailPage } from './KnowledgeDetailPage'
import { ChunkViewer } from './components/ChunkViewer'
import { DocumentImport } from './components/DocumentImport'
import { KnowledgeForm } from './components/KnowledgeForm'
import { knowledgeBase, knowledgeChunk, knowledgeDocument } from '../../../tests/fixtures/knowledge'
import { aiFixture } from '../../../tests/fixtures/ai'

const response = (data: unknown) => new Response(JSON.stringify({ data }))
beforeEach(() => {
  Object.defineProperty(HTMLDialogElement.prototype, 'showModal', { configurable: true, value() { this.setAttribute('open', '') } })
  Object.defineProperty(HTMLDialogElement.prototype, 'close', { configurable: true, value() { this.removeAttribute('open') } })
})
afterEach(() => { cleanup(); vi.unstubAllGlobals() })
function boot() {
  return render(<MemoryRouter initialEntries={['/knowledge/42']}><Routes><Route path="/knowledge/:id" element={<KnowledgeDetailPage api={createApi({ token: 'tenant-token' })} />} /></Routes></MemoryRouter>)
}

test('导入手动文档后自动刷新处理状态，完成后显示分块数量', async () => {
  let submitted = false
  let reads = 0
  let title = ''
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (options.method === 'POST') {
      submitted = true
      title = JSON.parse(String(options.body)).title
      return response({ ...knowledgeDocument, title, parse_status: 'queued' })
    }
    if (url.endsWith('/documents')) {
      if (!submitted) return response([])
      reads += 1
      return response([{ ...knowledgeDocument, title, parse_status: reads > 1 ? 'completed' : 'queued', chunk_count: reads > 1 ? 1 : 0 }])
    }
    return response({ ...knowledgeBase, document_count: submitted ? 1 : 0 })
  })
  const view = boot()
  fireEvent.click(await screen.findByRole('button', { name: '添加文档' }))
  fireEvent.click(screen.getByRole('tab', { name: '手动输入' }))
  fireEvent.change(screen.getByLabelText('文档标题'), { target: { value: '新休假制度' } })
  fireEvent.change(screen.getByLabelText('正文内容'), { target: { value: knowledgeChunk.content } })
  fireEvent.click(screen.getByRole('button', { name: '添加并处理' }))
  expect(await screen.findByRole('button', { name: '新休假制度' })).toBeTruthy()
  expect(await screen.findByText('等待处理')).toBeTruthy()
  await waitFor(() => expect(screen.getByText('已完成')).toBeTruthy(), { timeout: 4000 })
  expect(reads).toBe(2)
  view.unmount()
})

test('检索显示真实分块、来源和得分，外部内容按普通文本展示', async () => {
  const text = '<img src=x onerror=alert(1)> 年假需要提前三天申请'
  vi.stubGlobal('fetch', async (url: string) => {
    if (url.endsWith('/search')) return response([{ ...knowledgeChunk, content: text, document_title: '休假制度', file_name: '手册.txt', score: .875 }])
    if (url.endsWith('/documents')) return response([knowledgeDocument])
    return response(knowledgeBase)
  })
  boot()
  fireEvent.click(await screen.findByRole('tab', { name: '检索测试' }))
  fireEvent.change(screen.getByLabelText('检索内容'), { target: { value: '年假' } })
  fireEvent.click(screen.getByRole('button', { name: '开始检索' }))
  expect(await screen.findByText(text)).toBeTruthy()
  expect(screen.getByText('得分 0.8750')).toBeTruthy()
  expect(screen.getByText(/来源：手册.txt/)).toBeTruthy()
  expect(screen.queryByRole('img')).toBeNull()
})

test('连续检索时输入框标签保持不变，空结果后可输入新的查询', async () => {
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (url.endsWith('/search')) {
      const body: unknown = JSON.parse(String(options.body))
      const query = typeof body === 'object' && body !== null && 'query' in body ? body.query : undefined
      return response(query === '年假' ? [{ ...knowledgeChunk, document_title: knowledgeDocument.title, file_name: '', score: .9 }] : [])
    }
    if (url.endsWith('/documents')) return response([knowledgeDocument])
    return response(knowledgeBase)
  })
  boot()
  fireEvent.click(await screen.findByRole('tab', { name: '检索测试' }))
  fireEvent.change(screen.getByLabelText('检索内容', { exact: true }), { target: { value: '航星' } })
  fireEvent.click(screen.getByRole('button', { name: '开始检索' }))
  expect(await screen.findByRole('heading', { name: '未检索到匹配内容' })).toBeTruthy()
  // RTL 会忽略 label 内的 textarea 文本，浏览器标签定位还需依赖独立的固定名称。
  expect(screen.getByRole('textbox', { name: '检索内容' }).getAttribute('aria-label')).toBe('检索内容')
  fireEvent.change(screen.getByLabelText('检索内容', { exact: true }), { target: { value: '年假' } })
  fireEvent.click(screen.getByRole('button', { name: '开始检索' }))
  expect(await screen.findByText(knowledgeChunk.content)).toBeTruthy()
  expect(screen.getByRole('textbox', { name: '检索内容' })).toHaveProperty('value', '年假')
})

test('分块编辑保存后确认重新索引，并刷新已保存的内容', async () => {
  let content = knowledgeChunk.content
  vi.stubGlobal('fetch', async (_url: string, options: RequestInit) => {
    if (options.method === 'PATCH') {
      content = JSON.parse(String(options.body)).content
      return response({ ...knowledgeDocument, parse_status: 'queued' })
    }
    return response([{ ...knowledgeChunk, content }])
  })
  const changed = vi.fn()
  render(<ChunkViewer api={createApi({})} document={knowledgeDocument} onClose={vi.fn()} onChanged={changed} />)
  fireEvent.click(await screen.findByRole('button', { name: '编辑' }))
  fireEvent.change(screen.getByLabelText('分块 1 内容'), { target: { value: '年假申请提前五天提交。' } })
  fireEvent.click(screen.getByRole('button', { name: '保存分块' }))
  expect(await screen.findByText('分块已保存，文档正在重新建立索引。')).toBeTruthy()
  expect(await screen.findByText('年假申请提前五天提交。')).toBeTruthy()
  expect(changed).toHaveBeenCalledOnce()
  expect(screen.getByText(/分块编辑不修改原件与原文/)).toBeTruthy()
})

test('文件导入校验 PDF 解析配置，文本按 multipart 上传且不覆盖 boundary', async () => {
  let multipart: FormData | undefined
  vi.stubGlobal('fetch', async (_url: string, options: RequestInit) => {
    if (options.body instanceof FormData) multipart = options.body
    expect(options.headers).not.toHaveProperty('Content-Type')
    return response({ ...knowledgeDocument, source: 'file', file_name: '制度.txt' })
  })
  const saved = vi.fn()
  render(<DocumentImport api={createApi({})} base={knowledgeBase} onClose={vi.fn()} onSaved={saved} />)
  const pdf = new File(['pdf'], '制度.pdf', { type: 'application/pdf' })
  fireEvent.change(screen.getByLabelText('选择文件'), { target: { files: { item: () => pdf } } })
  fireEvent.click(screen.getByRole('button', { name: '上传并处理' }))
  expect((await screen.findByRole('alert')).textContent).toContain('配置 PDF 解析服务地址')
  expect(saved).not.toHaveBeenCalled()
  const txt = new File(['申请年假'], '制度.txt', { type: 'text/plain' })
  fireEvent.change(screen.getByLabelText('选择文件'), { target: { files: { item: () => txt } } })
  fireEvent.click(screen.getByRole('button', { name: '上传并处理' }))
  await waitFor(() => expect(saved).toHaveBeenCalledOnce())
  expect(multipart?.get('file')).toHaveProperty('name', '制度.txt')
})

test('已有资料修改处理配置须明确确认重建，写入只包含配置字段', async () => {
  const writes: { url: string; body: unknown }[] = []
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (options.method === 'POST') {
      writes.push({ url, body: JSON.parse(String(options.body)) })
      return response({ ...knowledgeBase, parser_endpoint: 'http://docreader:8080', processing_count: 1 })
    }
    return response(aiFixture(url))
  })
  const saved = vi.fn()
  render(<MemoryRouter><KnowledgeForm api={createApi({})} base={knowledgeBase} onClose={vi.fn()} onSaved={saved} /></MemoryRouter>)
  fireEvent.change(await screen.findByLabelText('PDF 解析服务地址', { exact: false }), { target: { value: 'http://docreader:8080' } })
  fireEvent.click(screen.getByRole('button', { name: '保存并重建全部文档' }))
  expect((await screen.findByRole('alert')).textContent).toContain('请确认重新处理')
  expect(writes).toHaveLength(0)
  fireEvent.click(screen.getByLabelText('确认重建当前知识库的全部资料'))
  fireEvent.click(screen.getByRole('button', { name: '保存并重建全部文档' }))
  await waitFor(() => expect(saved).toHaveBeenCalledOnce())
  expect(writes).toEqual([{ url: '/api/knowledge/bases/42/rebuild', body: {
    name: knowledgeBase.name, description: knowledgeBase.description, indexing_mode: 'keyword', retrieval_mode: 'keyword',
    embedding_model_id: '', vector_store_id: '', parser_endpoint: 'http://docreader:8080', chunk_size: 512, chunk_overlap: 80, chunk_strategy: 'auto',
  } }])
})

test('名称变更直接保存，正在处理资料时阻止发出重建请求', async () => {
  const writes: string[] = []
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    if (options.method === 'PATCH') { writes.push(url); return response({ ...knowledgeBase, name: '新的名称' }) }
    return response(aiFixture(url))
  })
  render(<MemoryRouter><KnowledgeForm api={createApi({})} base={{ ...knowledgeBase, processing_count: 1 }} onClose={vi.fn()} onSaved={vi.fn()} /></MemoryRouter>)
  fireEvent.change(await screen.findByLabelText('知识库名称'), { target: { value: '新的名称' } })
  fireEvent.click(screen.getByRole('button', { name: '保存设置' }))
  await waitFor(() => expect(writes).toEqual(['/api/knowledge/bases/42']))
  fireEvent.change(screen.getByLabelText('分块长度', { exact: false }), { target: { value: '1024' } })
  expect(screen.getByRole('button', { name: '保存并重建全部文档' })).toHaveProperty('disabled', true)
})
