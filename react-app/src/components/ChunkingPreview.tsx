import { useState } from 'react'
import type { Api } from '../api/client'
import type { ChunkPreview } from '../api/knowledge/types'
import { ErrorNotice, Field } from './ui'
import { useAction } from '../hooks/async'
import './ChunkingPreview.css'

/** 预览只处理输入的样本文字，不创建文档或修改现有索引。 */
export function ChunkingPreview({ api, strategy, size, overlap }: { api: Api; strategy: string; size: number; overlap: number }) {
  const [content, setContent] = useState('')
  const scope = `${strategy}/${size}/${overlap}/${content}`
  const [result, setResult] = useState<{ scope: string; chunks: ChunkPreview[] } | null>(null)
  const chunks = result?.scope === scope ? result.chunks : null
  const action = useAction(scope)
  return <div className="chunking-preview">
    <Field label="切分预览样本"><textarea aria-label="切分预览样本" rows={4} value={content} maxLength={100000} onChange={event => setContent(event.target.value)} placeholder="粘贴一段具有代表性的文档，检查分块效果。" /></Field>
    <button type="button" className="button secondary" disabled={action.busy || !content.trim()} onClick={() => { void action.run(signal => api.previewChunks({ content, chunk_strategy: strategy, chunk_size: size, chunk_overlap: overlap }, signal), chunks => setResult({ scope, chunks })) }}>{action.busy ? '正在预览…' : '预览分块'}</button>
    <ErrorNotice message={action.error} />
    {chunks && <div className="chunk-preview-results" aria-live="polite"><p>共 {chunks.length} 个分块</p>{chunks.map(chunk => <article key={chunk.index}><small>分块 {chunk.index + 1} · 字符 {chunk.start}–{chunk.end}</small>{chunk.context_header && <strong>{chunk.context_header}</strong>}<pre>{chunk.content}</pre></article>)}</div>}
  </div>
}
