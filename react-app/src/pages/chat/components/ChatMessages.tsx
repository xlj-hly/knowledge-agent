import { useState } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type { Message } from '../../../api/chat/types'
import { Icon } from '../../../components/ui'

export function ChatMessages({ messages, onSources }: { messages: Message[]; onSources: (message: Message) => void }) {
  const [copied, setCopied] = useState('')
  const [copyError, setCopyError] = useState('')
  const copy = async (message: Message) => {
    try { await navigator.clipboard.writeText(message.content); setCopied(message.id); setCopyError('') }
    catch { setCopyError('复制失败，请选中回答文字后复制。') }
  }
  return <>{messages.map(message => <article key={message.id} className={`chat-message ${message.role}`}>
    <div className="chat-message-avatar"><Icon name={message.role === 'user' ? 'user' : 'spark'} size={19} /></div>
    <div className="chat-message-body">
      <div className="chat-message-author">{message.role === 'user' ? '你' : '助手'}<small role={message.status === 'generating' ? 'status' : undefined}>{message.status === 'generating' ? '正在回答…' : message.status === 'failed' ? '生成失败' : message.status === 'cancelled' ? '已停止' : ''}</small></div>
      <div className="chat-message-content">{message.role === 'user' ? message.content : message.content ? <ReactMarkdown remarkPlugins={[remarkGfm]} skipHtml components={{ a: ({ children, href }) => <a href={href} target="_blank" rel="noreferrer">{children}</a> }}>{message.content}</ReactMarkdown> : <span className="chat-generation-placeholder">{message.status === 'generating' ? '正在检索资料并组织回答…' : message.status === 'failed' ? '本次回答未完成，可重新提问。' : ''}</span>}</div>
      {message.role === 'assistant' && <div className="chat-message-actions">
        {message.sources.length > 0 && <button type="button" className="chat-source-button" onClick={() => onSources(message)}><Icon name="file" size={14} />参考资料 · {message.sources.length}</button>}
        {message.content && message.status !== 'generating' && <button type="button" className="chat-copy-button" onClick={() => { void copy(message) }} aria-label={copied === message.id ? '已复制回答' : '复制回答'}><Icon name={copied === message.id ? 'check' : 'copy'} size={14} />{copied === message.id ? '已复制' : '复制'}</button>}
      </div>}
    </div>
  </article>)}{copyError && <p role="alert" className="chat-copy-error">{copyError}</p>}</>
}
