import { useCallback, useState } from 'react'
import { Drawer, Dropdown } from 'antd'
import { Link, useNavigate, useParams } from 'react-router'
import type { Api } from '../../api/client'
import type { AISettings, Presets } from '../../api/ai/types'
import type { KnowledgeBase } from '../../api/knowledge/types'
import { ErrorNotice, Icon, Loading } from '../../components/ui'
import { ConfirmDelete } from '../../components/ui/ConfirmDelete'
import { useAction, useResource } from '../../hooks/async'
import { useConversation } from './hooks/useConversation'
import { useChatScroll } from './hooks/useChatScroll'
import { ConversationSettings } from './components/ConversationSettings'
import { ChatComposer } from './components/ChatComposer'
import { ChatMessages } from './components/ChatMessages'
import { ChatHistorySidebar } from './components/ChatHistorySidebar'
import { SourcesDrawer } from './components/SourcesDrawer'
import './chat.css'

export function ChatPage({ api }: { api: Api }) {
  const catalog = useResource(useCallback(async (signal: AbortSignal) => {
    const [bases, settings, presets] = await Promise.all([api.knowledgeBases(signal), api.aiSettings(signal), api.aiPresets(signal)])
    return { bases, settings, presets }
  }, [api]))
  if (catalog.error) return <ErrorNotice message={catalog.error} retry={catalog.refresh} />
  if (!catalog.data) return <Loading label="正在准备对话…" />
  return <ChatWorkspace api={api} {...catalog.data} />
}

function ChatWorkspace({ api, bases, settings, presets }: { api: Api; bases: KnowledgeBase[]; settings: AISettings; presets: Presets }) {
  const { id } = useParams()
  const navigate = useNavigate()
  const sessions = useResource(useCallback((signal: AbortSignal) => api.chatSessions(signal), [api]))
  const onCreated = useCallback((next: string) => { navigate(`/chat/${next}`, { replace: true }) }, [navigate])
  const conversation = useConversation(api, id, settings, onCreated, sessions.refresh)
  const [input, setInput] = useState('')
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [historyOpen, setHistoryOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [sourceOpen, setSourceOpen] = useState<string | null>(null)
  const [routeId, setRouteId] = useState(id)
  if (routeId !== id) {
    setRouteId(id)
    // 新建会话的路由提交可晚于首条回答，不能把同一会话的交互状态清掉。
    if (id !== conversation.session?.id) {
      setInput(''); setSourceOpen(null); setSettingsOpen(false); setDeleting(false); setHistoryOpen(false)
    }
  }
  const action = useAction(id)
  const { viewport, away, onScroll, toBottom } = useChatScroll(id, conversation.messages)
  const empty = !conversation.loading && !conversation.messages.length
  const newChat = () => { conversation.reset(); setHistoryOpen(false); setInput(''); setSourceOpen(null); navigate('/'); sessions.refresh() }
  const send = () => {
    const value = input
    toBottom()
    void conversation.send(value, () => setInput(current => current === value ? '' : current))
  }
  const historyProps = { sessions: sessions.data ?? [], current: id, loading: sessions.loading, error: sessions.error, onRefresh: sessions.refresh, onNew: newChat, onSelect: () => setHistoryOpen(false) }
  return <div className="chat-workspace">
    <div className="chat-desktop-history"><ChatHistorySidebar {...historyProps} /></div>
    <section className={`chat-main${empty ? ' empty' : ''}`} aria-label="对话区域">
      <header className="chat-toolbar">
        <div className="chat-current-title"><button className="icon-button chat-history-toggle" aria-label="打开历史对话" onClick={() => setHistoryOpen(true)}><Icon name="menu" size={20} /></button><span>{conversation.config.title || '新对话'}</span></div>
        <Dropdown trigger={['click']} menu={{ items: [{ key: 'settings', label: '当前对话设置', icon: <Icon name="settings" size={15} /> }, ...(id ? [{ key: 'delete', label: '删除当前对话', danger: true, icon: <Icon name="trash" size={15} /> }] : [])], onClick: ({ key }) => { if (key === 'settings') setSettingsOpen(true); if (key === 'delete') setDeleting(true) } }}>
          <button className="icon-button" aria-label="对话操作" disabled={conversation.streaming || conversation.loading}><Icon name="more" size={20} /></button>
        </Dropdown>
      </header>
      <div className="chat-stage">
        {empty ? <div className="chat-welcome"><span className="chat-welcome-icon"><Icon name="spark" size={30} /></span><h1>今天想了解什么？</h1><p>从一个问题开始，让知识触手可及。</p></div> : <div ref={viewport} onScroll={onScroll} className="chat-message-viewport" role="log" aria-label="消息列表" aria-relevant="additions text"><div className="chat-message-list">{conversation.loading ? <Loading label="正在读取对话…" /> : <ChatMessages messages={conversation.messages} onSources={message => setSourceOpen(message.id)} />}</div></div>}
        {away && !empty && <button className="chat-scroll-bottom" aria-label="回到底部" onClick={toBottom}><Icon name="arrow" size={16} /></button>}
        <div className="chat-composer-area">
          {empty && !settings.chat_model_id && <div className="chat-setup-notice">尚未启用对话模型，请企业管理员在 <Link to="/models">LLM 对话模型</Link> 中配置并启用。</div>}
          <ErrorNotice message={conversation.error || action.error} retry={id ? () => { void conversation.refresh() } : undefined} />
          <ChatComposer input={input} onInput={setInput} onSend={send} onStop={() => { void conversation.stop() }} loading={conversation.loading} streaming={conversation.streaming} bases={bases} selected={conversation.config.knowledge_base_ids} onSelect={knowledge_base_ids => conversation.setConfig(current => ({ ...current, knowledge_base_ids }))} />
          <p className="chat-footnote">{conversation.config.knowledge_base_ids.length ? '回答将结合所选知识库，并展示参考资料' : 'AI 生成内容，请核对重要信息'}</p>
        </div>
      </div>
    </section>
    <Drawer title="我的对话" placement="left" open={historyOpen} onClose={() => setHistoryOpen(false)} size={280} destroyOnHidden className="chat-history-drawer"><ChatHistorySidebar {...historyProps} /></Drawer>
    <SourcesDrawer message={conversation.messages.find(message => message.id === sourceOpen)} onClose={() => setSourceOpen(null)} />
    {settingsOpen && <ConversationSettings config={conversation.config} presets={presets} busy={action.busy} error={action.error} onClose={() => setSettingsOpen(false)} onSaved={config => { if (!id) { conversation.setConfig(config); setSettingsOpen(false); return } void action.run(signal => api.saveChatSession(id, config, signal), saved => { conversation.setConfig(saved); setSettingsOpen(false); sessions.refresh() }) }} />}
    {deleting && id && <ConfirmDelete title="删除当前对话？" description="此对话及消息记录将被删除，知识库中的资料不受影响。" onDelete={signal => api.deleteChatSession(id, signal)} onClose={() => setDeleting(false)} onDeleted={() => { setDeleting(false); navigate('/'); sessions.refresh() }} />}
  </div>
}
