import { useCallback, useState } from 'react'
import { Link, useNavigate } from 'react-router'
import type { Api } from '../../api/client'
import { EmptyState, ErrorNotice, Icon, Loading, PageHeader, SearchForm } from '../../components/ui'
import { useResource } from '../../hooks/async'
import { KnowledgeForm } from './components/KnowledgeForm'
import { modeLabels } from './labels'
import './knowledge.css'

export function KnowledgePage({ api }: { api: Api }) {
  const list = useResource(useCallback((signal: AbortSignal) => api.knowledgeBases(signal), [api]))
  const [query, setQuery] = useState('')
  const [creating, setCreating] = useState(false)
  const navigate = useNavigate()
  const items = list.data?.filter(item => `${item.name} ${item.description}`.toLowerCase().includes(query.toLowerCase())) ?? []
  return <div className="knowledge-page">
    <PageHeader eyebrow="KNOWLEDGE BASE" title="知识库" description="把分散的资料变为可检索的企业知识。"
      action={<button className="button primary" onClick={() => setCreating(true)}><Icon name="plus" size={17} />新建知识库</button>} />
    <section className="panel knowledge-library">
      <div className="knowledge-toolbar"><div><h2>全部知识库 <span className="count-badge">{list.data?.length ?? 0}</span></h2><p>导入文档、查看分块，并验证检索效果。</p></div>
        <div className="toolbar-actions"><SearchForm placeholder="搜索知识库名称或描述" onSearch={setQuery} />
          <button className="icon-button" aria-label="刷新知识库" onClick={list.refresh}><Icon name="refresh" size={17} /></button></div></div>
      <ErrorNotice message={list.error} retry={list.refresh} />
      {list.loading ? <Loading /> : list.data && (!items.length ? <EmptyState title={query ? '没有匹配的知识库' : '还没有知识库'} description={query ? '尝试其他名称或描述。' : '新建知识库，上传文档或手动添加内容。'} /> :
        <div className="knowledge-grid">{items.map(item => <Link className="knowledge-card" key={item.id} to={`/knowledge/${item.id}`}>
          <div className="knowledge-card-top"><span className="knowledge-book"><Icon name="book" size={24} /></span><span className="badge gray">{modeLabels[item.indexing_mode]}</span></div>
          <h3>{item.name}</h3><p>{item.description || '尚未填写描述'}</p>
          <div className="knowledge-card-footer"><span>{item.document_count} 篇文档 · {item.chunk_count} 个分块</span><Icon name="arrow" size={17} /></div>
        </Link>)}</div>)}
    </section>
    {creating && <KnowledgeForm api={api} onClose={() => setCreating(false)} onSaved={base => navigate(`/knowledge/${base.id}`)} />}
  </div>
}
