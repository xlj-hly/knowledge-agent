import { useLayoutEffect, useRef } from 'react'
import { Popover } from 'antd'
import { Link } from 'react-router'
import type { KnowledgeBase } from '../../../api/knowledge/types'
import { Icon } from '../../../components/ui'

export function ChatComposer({ input, onInput, onSend, onStop, loading, streaming, bases, selected, onSelect }: {
  input: string; onInput: (value: string) => void; onSend: () => void; onStop: () => void
  loading: boolean; streaming: boolean; bases: KnowledgeBase[]; selected: string[]; onSelect: (ids: string[]) => void
}) {
  const textarea = useRef<HTMLTextAreaElement>(null)
  const composing = useRef(false)
  useLayoutEffect(() => {
    if (!textarea.current) return
    textarea.current.style.height = 'auto'
    textarea.current.style.height = `${Math.min(textarea.current.scrollHeight, 192)}px`
  }, [input])
  const unavailable = selected.filter(value => !bases.some(base => base.id === value))
  const knowledgePicker = <div className="chat-knowledge-popover">
    <strong>使用知识库</strong><p>不选择时进行普通对话</p>
    <fieldset disabled={loading || streaming} className="chat-knowledge-options">
      {bases.map(base => <label key={base.id}><input type="checkbox" checked={selected.includes(base.id)} onChange={event => onSelect(event.target.checked ? [...selected, base.id] : selected.filter(id => id !== base.id))} /><span>{base.name}<small>{base.document_count} 篇资料</small></span></label>)}
      {unavailable.map(id => <label key={id}><input type="checkbox" checked onChange={() => onSelect(selected.filter(value => value !== id))} /><span>已删除的知识库（ID：{id}）<small>请取消选择后继续对话</small></span></label>)}
      {!bases.length && <p>还没有可用知识库，<Link to="/knowledge">先添加资料</Link>。</p>}
    </fieldset>
  </div>
  return <form className="chat-composer" onSubmit={event => { event.preventDefault(); if (!loading && !streaming && input.trim()) onSend() }}>
    <textarea ref={textarea} aria-label="输入消息" rows={1} value={input} disabled={loading} onChange={event => onInput(event.target.value)} onCompositionStart={() => { composing.current = true }} onCompositionEnd={() => { composing.current = false }} onKeyDown={event => {
      if (event.key !== 'Enter' || event.shiftKey || composing.current || event.nativeEvent.isComposing || event.keyCode === 229) return
      event.preventDefault()
      if (!loading && !streaming && input.trim()) onSend()
    }} placeholder="发送消息，或选择知识库提问…" maxLength={selected.length ? 2000 : 16000} />
    <div className="chat-composer-footer">
      <Popover content={knowledgePicker} trigger="click" placement="topLeft" destroyOnHidden>
        <button type="button" className={`chat-knowledge-trigger${selected.length ? ' selected' : ''}`} aria-label="选择知识库" disabled={loading || streaming}><Icon name="book" size={17} />{selected.length ? `${selected.length} 个知识库` : '知识库'}<span className="chat-picker-chevron"><Icon name="chevron" size={12} /></span></button>
      </Popover>
      <div className="chat-send-actions"><span className="chat-keyboard-hint">Shift + Enter 换行</span>{streaming ? <button type="button" className="chat-send-button stopping" aria-label="停止生成" title="停止生成" onClick={onStop}><Icon name="stop" size={17} /></button> : <button type="submit" className="chat-send-button" aria-label="发送" title="发送" disabled={loading || !input.trim()}><Icon name="arrow" size={20} /></button>}</div>
    </div>
  </form>
}
