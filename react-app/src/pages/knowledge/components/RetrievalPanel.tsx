import { useState } from 'react'
import type { Api } from '../../../api/client'
import type { IndexingMode, KnowledgeBase, SearchResult } from '../../../api/knowledge/types'
import { EmptyState, ErrorNotice, Field, Icon } from '../../../components/ui'
import { useAction } from '../../../hooks/async'
import { modeLabels } from '../labels'

export function RetrievalPanel({ api, base, onOpenDocument }: { api: Api; base: KnowledgeBase; onOpenDocument: (id: string) => void }) {
  const [query, setQuery] = useState('')
  const [mode, setMode] = useState<IndexingMode>(base.retrieval_mode)
  const [limit, setLimit] = useState(5)
  const [results, setResults] = useState<SearchResult[] | null>(null)
  const [validation, setValidation] = useState('')
  const action = useAction(base.id)
  return <>
    <div className="knowledge-section-heading"><div><h2>检索测试</h2><p>输入问题或关键词，检查资料分块的命中情况和来源。</p></div></div>
    <form className="knowledge-retrieval-form" onSubmit={event => {
      event.preventDefault()
      if (!query.trim()) { setValidation('请输入检索内容'); return }
      setValidation(''); setResults(null)
      void action.run(signal => api.searchKnowledge(base.id, { query: query.trim(), mode, limit }, signal), setResults)
    }}><Field label="检索内容"><textarea aria-label="检索内容" rows={3} value={query} disabled={action.busy} onChange={event => setQuery(event.target.value)} placeholder="例如：如何申请年假？" /></Field>
      <div className="knowledge-retrieval-options"><Field label="检索模式"><select aria-label="检索模式" value={mode} disabled={action.busy} onChange={event => { const value = event.target.value; if (value === 'keyword' || value === 'vector' || value === 'hybrid') setMode(value) }}>
        {(base.indexing_mode === 'keyword' ? ['keyword'] as const : ['keyword', 'vector', 'hybrid'] as const).map(value => <option key={value} value={value}>{modeLabels[value]}</option>)}
      </select></Field><Field label="返回数量"><select aria-label="返回数量" value={limit} disabled={action.busy} onChange={event => setLimit(Number(event.target.value))}>{[3, 5, 10, 20].map(value => <option key={value} value={value}>{value} 条</option>)}</select></Field>
        <button className="button primary" disabled={action.busy}><Icon name="search" size={17} />{action.busy ? '正在检索…' : '开始检索'}</button></div>
    </form>
    <ErrorNotice message={validation || action.error} />
    {results && <div className="knowledge-results" aria-live="polite">{!results.length ? <EmptyState title="未检索到匹配内容" description="尝试其他关键词，并确认文档已处理完成。" /> : <><p className="muted">命中 {results.length} 个分块</p>{results.map((result, index) => <article className="knowledge-result" key={result.id}>
      <div className="knowledge-result-header"><button className="knowledge-document-title" onClick={() => onOpenDocument(result.document_id)}>{index + 1}. {result.document_title}</button><span>得分 {result.score.toFixed(4)}</span></div>{result.context_header && <strong>{result.context_header}</strong>}<p>{result.content}</p><small>来源：{result.file_name || '手动输入'} · 分块 {result.index + 1} · 字符 {result.start}–{result.end}</small>
    </article>)}</>}</div>}
  </>
}
