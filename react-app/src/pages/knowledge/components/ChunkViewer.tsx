import { useCallback, useState } from 'react'
import type { Api } from '../../../api/client'
import type { Chunk, KnowledgeDocument } from '../../../api/knowledge/types'
import { EmptyState, ErrorNotice, Loading, Modal, SuccessNotice } from '../../../components/ui'
import { useAction, useResource } from '../../../hooks/async'

export function ChunkViewer({ api, document, onClose, onChanged }: { api: Api; document: KnowledgeDocument; onClose: () => void; onChanged: () => void }) {
  const chunks = useResource(useCallback((signal: AbortSignal) => api.knowledgeChunks(document.id, signal), [api, document.id]))
  const [editing, setEditing] = useState<Chunk | null>(null)
  const [content, setContent] = useState('')
  const [notice, setNotice] = useState('')
  const [validation, setValidation] = useState('')
  const action = useAction(document.id)
  return <Modal title={document.title} description="查看处理后的分块；编辑分块会重新建立整篇文档的索引。" onClose={onClose} busy={action.busy}>
    <div className="knowledge-page modal-form">
      <p className="knowledge-hint">分块编辑不修改原件与原文；「重新处理」会重新切分原文，覆盖分块编辑。索引完成后新内容才参与检索。</p>
      <div className="knowledge-section-heading"><span className="muted">{chunks.data?.length ?? 0} 个分块</span><button className="button secondary" disabled={action.busy || editing !== null} onClick={chunks.refresh}>刷新分块</button></div>
      <SuccessNotice message={notice} /><ErrorNotice message={validation || action.error || chunks.error} retry={chunks.error ? chunks.refresh : undefined} />
      {chunks.loading ? <Loading /> : chunks.data && (!chunks.data.length ? <EmptyState title="暂无可查看的分块" description="等待资料处理完成，或在失败后重新处理。" /> : <div className="knowledge-chunks">{chunks.data.map(item => <article className="knowledge-chunk" key={item.id}>
        <div className="knowledge-chunk-heading"><span>分块 {item.index + 1} · 字符 {item.start}–{item.end}</span><button className="button secondary" disabled={action.busy || (editing !== null && editing.id !== item.id) || document.parse_status !== 'completed'} onClick={() => { setEditing(item); setContent(item.content); setNotice(''); setValidation('') }}>编辑</button></div>
        {item.context_header && <strong className="knowledge-context-header">{item.context_header}</strong>}
        {editing?.id === item.id ? <form onSubmit={event => {
          event.preventDefault()
          if (!content.trim()) { setValidation('分块内容不能为空'); return }
          if (new TextEncoder().encode(content).length > 65536) { setValidation('分块内容不能超过 64 KiB'); return }
          setValidation('')
          void action.run(signal => api.updateKnowledgeChunk(item.id, content, signal), () => { setEditing(null); setNotice('分块已保存，文档正在重新建立索引。'); chunks.refresh(); onChanged() })
        }}><textarea aria-label={`分块 ${item.index + 1} 内容`} disabled={action.busy} value={content} onChange={event => setContent(event.target.value)} /><div className="knowledge-inline-actions"><button type="button" className="button secondary" disabled={action.busy} onClick={() => setEditing(null)}>取消编辑</button><button className="button primary" disabled={action.busy}>{action.busy ? '正在保存…' : '保存分块'}</button></div></form> : <p>{item.content}</p>}
      </article>)}</div>)}
    </div>
  </Modal>
}
