import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import type { Message } from '../../../api/chat/types'
import { ChatMessages } from './ChatMessages'

const answer: Message = { id: '101', session_id: '10', role: 'assistant', content: '', status: 'completed', sources: [], created_at: '2026-11-07T00:00:00Z' }
afterEach(() => { cleanup(); vi.unstubAllGlobals() })

test('流式与历史回答统一渲染 Markdown 标题、代码和表格，用户原文不作 Markdown 解释', () => {
  const content = '# 操作说明\n\n```go\nfmt.Println("hello")\n```\n\n| 项目 | 结果 |\n| --- | --- |\n| 审批 | 已通过 |'
  const view = render(<ChatMessages messages={[{ ...answer, content, status: 'generating' }, { ...answer, id: '102', role: 'user', content: '**原文问题**' }]} onSources={vi.fn()} />)
  expect(screen.getByRole('heading', { name: '操作说明', level: 1 })).toBeTruthy()
  expect(screen.getByRole('table').textContent).toContain('审批已通过')
  expect(view.container.querySelector('pre > code')?.textContent).toBe('fmt.Println("hello")\n')
  expect(screen.getByText('**原文问题**').querySelector('strong')).toBeNull()
  expect(screen.getByRole('status').textContent).toBe('正在回答…')
  expect(screen.queryByRole('button', { name: '复制回答' })).toBeNull()
  view.rerender(<ChatMessages messages={[{ ...answer, content }]} onSources={vi.fn()} />)
  expect(screen.getByRole('heading', { name: '操作说明', level: 1 })).toBeTruthy()
  expect(screen.getByRole('table').textContent).toContain('审批已通过')
  expect(screen.getByRole('button', { name: '复制回答' })).toBeTruthy()
  expect(screen.queryByRole('status')).toBeNull()
})

test('回答不执行原始 HTML，链接拒绝 javascript 协议并保留正常 HTTPS 链接', () => {
  const content = '<script>window.pwned = true</script>\n\n<img src="x" onerror="window.pwned = true">\n\n[危险链接](javascript:alert%281%29)\n\n[正常文档](https://docs.example.test/help)'
  const view = render(<ChatMessages messages={[{ ...answer, content }]} onSources={vi.fn()} />)
  expect(view.container.querySelector('script')).toBeNull()
  expect(view.container.querySelector('img')).toBeNull()
  const rejected = screen.getByText('危险链接').closest('a')
  expect(rejected?.getAttribute('href') ?? '').not.toMatch(/^javascript:/i)
  expect(screen.getByRole('link', { name: '正常文档' }).getAttribute('href')).toBe('https://docs.example.test/help')
  expect(screen.getByRole('link', { name: '正常文档' }).getAttribute('rel')).toContain('noreferrer')
})

test('复制发送回答原文，失败明确反馈且可重试成功', async () => {
  const writeText = vi.fn().mockRejectedValueOnce(new DOMException('denied', 'NotAllowedError')).mockResolvedValueOnce(undefined)
  vi.stubGlobal('navigator', { clipboard: { writeText } })
  render(<ChatMessages messages={[{ ...answer, content: '**审批通过**\n\n依据：第 3 条。' }]} onSources={vi.fn()} />)
  fireEvent.click(screen.getByRole('button', { name: '复制回答' }))
  expect(await screen.findByRole('alert')).toHaveProperty('textContent', '复制失败，请选中回答文字后复制。')
  expect(screen.queryByRole('button', { name: '已复制回答' })).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: '复制回答' }))
  expect(await screen.findByRole('button', { name: '已复制回答' })).toBeTruthy()
  await waitFor(() => expect(screen.queryByRole('alert')).toBeNull())
  expect(writeText).toHaveBeenNthCalledWith(1, '**审批通过**\n\n依据：第 3 条。')
  expect(writeText).toHaveBeenNthCalledWith(2, '**审批通过**\n\n依据：第 3 条。')
})

test('来源入口传递对应回答的真实来源快照', () => {
  const source = { base_id: '30', document_id: '40', chunk_id: '50', document_title: '审批规则', content: '提交后三日内审批。', score: 0.9 }
  const message = { ...answer, content: '将在三日内审批。[1]', sources: [source] }
  const onSources = vi.fn()
  render(<ChatMessages messages={[message]} onSources={onSources} />)
  fireEvent.click(screen.getByRole('button', { name: '参考资料 · 1' }))
  expect(onSources).toHaveBeenCalledWith(message)
})
