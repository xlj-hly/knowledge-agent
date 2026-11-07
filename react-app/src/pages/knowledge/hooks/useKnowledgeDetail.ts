import { useCallback, useEffect, useState } from 'react'
import type { Api } from '../../../api/client'
import type { KnowledgeBase, KnowledgeDocument } from '../../../api/knowledge/types'
import { errorMessage } from '../../../hooks/async'

/** 仅处理中的资料需要轮询；卸载和主动刷新取消旧请求及计时器，保留已展示的快照。 */
export function useKnowledgeDetail(api: Api, id: string) {
  const [revision, setRevision] = useState(0)
  const [snapshot, setSnapshot] = useState<{ base: KnowledgeBase; documents: KnowledgeDocument[] } | null>(null)
  const [error, setError] = useState('')
  const refresh = useCallback(() => setRevision(value => value + 1), [])
  useEffect(() => {
    const controller = new AbortController()
    const polling: { timer?: ReturnType<typeof setTimeout> } = {}
    const load = async () => {
      try {
        const [base, documents] = await Promise.all([api.knowledgeBase(id, controller.signal), api.knowledgeDocuments(id, controller.signal)])
        if (controller.signal.aborted) return
        setSnapshot({ base, documents })
        setError('')
        if (documents.some(item => item.parse_status === 'queued' || item.parse_status === 'processing' || item.parse_status === 'deleting')) polling.timer = setTimeout(() => { void load() }, 1500)
      } catch (cause) {
        if (!controller.signal.aborted) setError(errorMessage(cause))
      }
    }
    void load()
    return () => { controller.abort(); clearTimeout(polling.timer) }
  }, [api, id, revision])
  return { snapshot, error, refresh }
}
