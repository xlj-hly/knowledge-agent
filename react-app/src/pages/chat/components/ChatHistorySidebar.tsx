import { Link } from 'react-router'
import type { ChatSession } from '../../../api/chat/types'
import { ErrorNotice, Icon, Loading } from '../../../components/ui'

export function ChatHistorySidebar({ sessions, current, loading, error, onRefresh, onNew, onSelect }: { sessions: ChatSession[]; current?: string; loading: boolean; error: string; onRefresh: () => void; onNew: () => void; onSelect: () => void }) {
  return <aside className="chat-history">
    <div className="chat-history-brand"><Icon name="spark" size={21} /><strong>知识助手</strong></div>
    <button className="chat-new-button" onClick={onNew}><Icon name="plus" size={17} />新对话</button>
    <div className="chat-history-heading">最近对话<button className="icon-button" aria-label="刷新对话列表" onClick={onRefresh}><Icon name="refresh" size={14} /></button></div>
    <ErrorNotice message={error} retry={onRefresh} />
    {loading ? <Loading /> : !sessions.length ? <p className="chat-history-empty">开始一段对话，<br />随时回到这里继续。</p> : <nav aria-label="历史对话">{sessions.map(item => <Link key={item.id} to={`/chat/${item.id}`} onClick={onSelect} title={item.title} className={item.id === current ? 'active' : ''}><Icon name="chat" size={15} /><span>{item.title}</span></Link>)}</nav>}
    <div className="chat-history-bottom"><Icon name="book" size={16} /><span>连接知识，获得答案</span></div>
  </aside>
}
