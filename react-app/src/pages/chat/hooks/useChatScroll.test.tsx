import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { useChatScroll } from './useChatScroll'

let contentHeight = 1000
beforeEach(() => {
  contentHeight = 1000
  const positions = new WeakMap<Element, number>()
  // jsdom 没有布局，补足尺寸和浏览器原生滚动范围，仍挂载并执行真实 Hook。
  vi.spyOn(Element.prototype, 'clientHeight', 'get').mockReturnValue(300)
  vi.spyOn(Element.prototype, 'scrollHeight', 'get').mockImplementation(() => contentHeight)
  vi.spyOn(Element.prototype, 'scrollTop', 'get').mockImplementation(function (this: Element) { return positions.get(this) ?? 0 })
  vi.spyOn(Element.prototype, 'scrollTop', 'set').mockImplementation(function (this: Element, value: number) { positions.set(this, Math.max(0, Math.min(value, contentHeight - 300))) })
})
afterEach(() => { cleanup(); vi.restoreAllMocks() })

function Viewport({ scope = 'first', revision = 0 }: { scope?: string; revision?: number }) {
  const { viewport, away, onScroll, toBottom } = useChatScroll(scope, revision)
  return <>
    <div aria-label="消息窗口" ref={viewport} onScroll={onScroll} />
    <output>{away ? '正在阅读历史' : '正在跟随回答'}</output>
    <button onClick={toBottom}>回到底部</button>
  </>
}

test('默认跟随到底部，接近底部时新内容继续跟随', () => {
  const view = render(<Viewport />)
  const node = screen.getByLabelText<HTMLDivElement>('消息窗口')
  expect(node.scrollTop).toBe(700)
  node.scrollTop = 650
  fireEvent.scroll(node)
  expect(screen.getByText('正在跟随回答')).toBeTruthy()
  contentHeight = 1200
  view.rerender(<Viewport revision={1} />)
  expect(node.scrollTop).toBe(900)
})

test('上滚阅读历史不被新回答拉回，主动回底后恢复跟随', () => {
  const view = render(<Viewport />)
  const node = screen.getByLabelText<HTMLDivElement>('消息窗口')
  node.scrollTop = 200
  fireEvent.scroll(node)
  expect(screen.getByText('正在阅读历史')).toBeTruthy()
  contentHeight = 1300
  view.rerender(<Viewport revision={1} />)
  expect(node.scrollTop).toBe(200)
  expect(screen.getByText('正在阅读历史')).toBeTruthy()
  fireEvent.click(screen.getByRole('button', { name: '回到底部' }))
  expect(node.scrollTop).toBe(1000)
  expect(screen.getByText('正在跟随回答')).toBeTruthy()
  contentHeight = 1400
  view.rerender(<Viewport revision={2} />)
  expect(node.scrollTop).toBe(1100)
})

test('切换会话清除上滚状态并展示新会话底部', () => {
  const view = render(<Viewport />)
  const node = screen.getByLabelText<HTMLDivElement>('消息窗口')
  node.scrollTop = 100
  fireEvent.scroll(node)
  expect(screen.getByText('正在阅读历史')).toBeTruthy()
  contentHeight = 900
  view.rerender(<Viewport scope="second" revision={1} />)
  expect(node.scrollTop).toBe(600)
  expect(screen.getByText('正在跟随回答')).toBeTruthy()
})
