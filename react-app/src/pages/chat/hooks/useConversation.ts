import { useEffect, useRef, useState } from 'react'
import type { Api } from '../../../api/client'
import type { AISettings } from '../../../api/ai/types'
import type { ChatSession, Message, SessionInput } from '../../../api/chat/types'
import { ApiError } from '../../../api/ApiError'
import { errorMessage } from '../../../hooks/async'

export function conversationDefaults(settings: AISettings): SessionInput {
  return { title: '新对话', knowledge_base_ids: [], system_prompt: settings.system_prompt, temperature: settings.temperature, max_tokens: settings.max_tokens, history_turns: settings.history_turns, retrieval_mode: settings.retrieval_mode, retrieval_limit: settings.retrieval_limit, similarity_threshold: settings.similarity_threshold }
}

/** 会话切换取消旧流；断线后读取权威消息状态，不把客户端取消当作事务回滚。 */
export function useConversation(api: Api, id: string | undefined, settings: AISettings, onCreated: (id: string) => void, onChanged: () => void) {
  const [session, setSession] = useState<ChatSession | null>(null)
  const [config, setConfig] = useState<SessionInput>(() => conversationDefaults(settings))
  const [messages, setMessages] = useState<Message[]>([])
  const [loading, setLoading] = useState(Boolean(id))
  const [streaming, setStreaming] = useState(false)
  const [error, setError] = useState('')
  const [scopeId, setScopeId] = useState(id)
  if (scopeId !== id) {
    setScopeId(id)
    if (id !== session?.id) {
      setError(''); setStreaming(false); setMessages([]); setSession(null)
      setConfig(conversationDefaults(settings)); setLoading(Boolean(id))
    }
  }
  const active = useRef<string | undefined>(undefined)
  const createdNavigation = useRef<string | undefined>(undefined)
  const stream = useRef<AbortController | null>(null)
  const messageRevision = useRef(0)
  const mounted = useRef(true)
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; stream.current?.abort() } }, [])
  useEffect(() => {
    if (id && id === createdNavigation.current) { createdNavigation.current = undefined; return }
    stream.current?.abort(); stream.current = null
    messageRevision.current += 1
    active.current = id
    if (!id) return
    const controller = new AbortController()
    Promise.all([api.chatSession(id, controller.signal), api.chatMessages(id, controller.signal)]).then(([next, rows]) => {
      if (controller.signal.aborted) return
      setSession(next); setConfig(next); setMessages(rows); setLoading(false)
    }, cause => { if (!controller.signal.aborted) { setError(errorMessage(cause)); setLoading(false) } })
    return () => controller.abort()
  }, [api, id, settings])
  const pending = messages.some(message => message.status === 'generating')
  useEffect(() => {
    if (!id || streaming || !pending) return
    const controller = new AbortController()
    const schedule: { timer?: ReturnType<typeof setTimeout> } = {}
    const poll = async () => {
      try { const rows = await api.chatMessages(id, controller.signal); if (!controller.signal.aborted) { setMessages(rows); if (rows.some(message => message.status === 'generating')) schedule.timer = setTimeout(() => { void poll() }, 1500) } }
      catch (cause) { if (!controller.signal.aborted) setError(errorMessage(cause)) }
    }
    schedule.timer = setTimeout(() => { void poll() }, 1500)
    return () => { controller.abort(); clearTimeout(schedule.timer) }
  }, [api, id, pending, streaming])
  const refresh = async () => {
    if (!active.current || stream.current) return
    const owner = active.current
    const revision = ++messageRevision.current
    try { const rows = await api.chatMessages(owner); if (mounted.current && active.current === owner && messageRevision.current === revision) { setMessages(rows); setError('') } }
    catch (cause) { if (mounted.current && active.current === owner && messageRevision.current === revision) setError(errorMessage(cause)) }
  }
  const send = async (content: string, onAccepted?: () => void): Promise<boolean> => {
    if (stream.current || pending || !content.trim()) return false
    const controller = new AbortController()
    const revision = ++messageRevision.current
    const previousUsers = new Set(messages.filter(message => message.role === 'user').map(message => message.id))
    stream.current = controller; setStreaming(true); setError('')
    const operation = { owner: active.current, assistantId: '', finished: false, accepted: false, rejected: false }
    const acknowledgement = { delivered: false }
    const acknowledge = () => {
      if (!acknowledgement.delivered && mounted.current && stream.current === controller) { acknowledgement.delivered = true; onAccepted?.() }
    }
    try {
      const next = operation.owner ? await api.saveChatSession(operation.owner, config, controller.signal) : await api.createChatSession(config, controller.signal)
      if (controller.signal.aborted || !mounted.current) return false
      operation.owner = next.id; active.current = next.id; setSession(next); setConfig(next)
      if (!id) { createdNavigation.current = next.id; onCreated(next.id) }
      setMessages(rows => [...rows, { id: `pending-${Date.now()}`, session_id: next.id, role: 'user', content: content.trim(), status: 'completed', sources: [], created_at: new Date().toISOString() }])
      await api.sendChatMessage(next.id, content.trim(), event => {
        if (stream.current !== controller || !mounted.current) return
        if (event.type === 'message') { operation.accepted = true; acknowledge(); operation.assistantId = event.message.id; setMessages(rows => [...rows.filter(row => row.id !== event.message.id), event.message]) }
        if (event.type === 'sources') setMessages(rows => rows.map(row => row.id === operation.assistantId ? { ...row, sources: event.sources } : row))
        if (event.type === 'delta') setMessages(rows => rows.map(row => row.id === operation.assistantId ? { ...row, content: row.content + event.content } : row))
        if (event.type === 'done') { operation.finished = true; setMessages(rows => rows.map(row => row.id === event.message.id ? event.message : row)) }
        if (event.type === 'error') { operation.finished = true; setError(event.message) }
      }, controller.signal)
      if (!operation.finished && !controller.signal.aborted) setError('对话流提前结束，已重新读取保存状态。')
    } catch (cause) {
      if (cause instanceof ApiError && cause.status >= 400 && cause.status < 500) operation.rejected = true
      if (!controller.signal.aborted && mounted.current && stream.current === controller) setError(errorMessage(cause))
    }
    finally {
      if (stream.current === controller && mounted.current) {
        if (operation.owner && active.current === operation.owner) {
          // 确认持久化终态前保持发送忙碌，防止旧读取覆盖下一轮消息与草稿。
          try {
            const rows = await api.chatMessages(operation.owner)
            if (mounted.current && active.current === operation.owner && stream.current === controller && messageRevision.current === revision) {
              setMessages(rows)
              if (!operation.rejected && rows.some(row => row.role === 'user' && !previousUsers.has(row.id) && row.content === content.trim())) { operation.accepted = true; acknowledge() }
            }
          } catch (cause) { if (mounted.current && active.current === operation.owner && stream.current === controller) setError(errorMessage(cause)) }
          if (mounted.current && stream.current === controller) onChanged()
        }
        if (mounted.current && stream.current === controller) { stream.current = null; setStreaming(false) }
      }
    }
    return operation.accepted && mounted.current && messageRevision.current === revision
  }
  const stop = async () => {
    const owner = active.current
    if (!owner) return
    try { await api.stopChat(owner); if (active.current === owner) { stream.current?.abort(); if (!stream.current) await refresh() } }
    catch (cause) { if (mounted.current && active.current === owner) setError(errorMessage(cause)) }
  }
  const reset = () => {
    // 新对话即使仍停留首页也建立新作用域，失效尚未返回的创建和延迟导航。
    stream.current?.abort(); stream.current = null
    messageRevision.current += 1
    active.current = undefined; createdNavigation.current = undefined
    setSession(null); setMessages([]); setConfig(conversationDefaults(settings))
    setStreaming(false); setLoading(false); setError('')
  }
  return { session, config, setConfig, messages, loading, streaming: streaming || pending, error, send, stop, refresh, reset }
}
