import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import type { Api } from '../../api/client'
import type { KnowledgeDocument } from '../../api/knowledge/types'
import { EmptyState, ErrorNotice, Icon, Loading, PageHeader, SearchForm, SuccessNotice } from '../../components/ui'
import { ConfirmDelete } from '../../components/ui/ConfirmDelete'
import { useAction } from '../../hooks/async'
import { RouteErrorPage } from '../error/RouteErrorPage'
import { useKnowledgeDetail } from './hooks/useKnowledgeDetail'
import { KnowledgeForm } from './components/KnowledgeForm'
import { modeLabels } from './labels'
import { DocumentImport } from './components/DocumentImport'
import { ChunkViewer } from './components/ChunkViewer'
import { RetrievalPanel } from './components/RetrievalPanel'
import './knowledge.css'

const statuses = { queued: '等待处理', processing: '处理中', completed: '已完成', failed: '处理失败', deleting: '正在删除' }
type Dialog = { kind: 'import' } | { kind: 'edit' | 'chunks' | 'delete-document'; document: KnowledgeDocument } | { kind: 'settings' | 'delete-base' }

export function KnowledgeDetailPage({ api }: { api: Api }) {
  const { id } = useParams()
  if (!id || !/^[1-9]\d*$/.test(id)) return <RouteErrorPage home="/knowledge" />
  return <Detail key={id} api={api} id={id} />
}

function Detail({ api, id }: { api: Api; id: string }) {
  const detail = useKnowledgeDetail(api, id)
  const [tab, setTab] = useState<'documents' | 'search'>('documents')
  const [query, setQuery] = useState('')
  const [dialog, setDialog] = useState<Dialog | null>(null)
  const [notice, setNotice] = useState('')
  const action = useAction(id)
  const navigate = useNavigate()
  if (!detail.snapshot) return detail.error ? <ErrorNotice message={detail.error} retry={detail.refresh} /> : <Loading label="正在加载知识库…" />
  const { base, documents } = detail.snapshot
  const filtered = documents.filter(item => `${item.title} ${item.file_name}`.toLowerCase().includes(query.toLowerCase()))
  const complete = (message: string) => { setDialog(null); setNotice(message); detail.refresh() }
  return <div className="knowledge-page">
    <Link className="knowledge-back" to="/knowledge">‹ 返回知识库</Link>
    <PageHeader eyebrow="KNOWLEDGE BASE" title={base.name} description={base.description || '导入文档并测试企业知识检索效果。'} action={<div className="toolbar-actions"><button className="button secondary" onClick={() => setDialog({ kind: 'settings' })}><Icon name="settings" size={17} />知识库设置</button><button className="button primary" onClick={() => setDialog({ kind: 'import' })}><Icon name="plus" size={17} />添加文档</button></div>} />
    <div className="knowledge-meta"><span className="badge gray">{base.indexing_mode === 'keyword' ? '关键词索引' : '关键词与向量索引'}</span><span>默认{modeLabels[base.retrieval_mode]}</span><span>{base.document_count} 篇文档</span><span>{base.chunk_count} 个分块</span>{base.processing_count > 0 && <span>{base.processing_count} 篇正在处理</span>}<span>每块 {base.chunk_size} 字符 · 重叠 {base.chunk_overlap}</span><Link to="/">前往对话</Link></div>
    <SuccessNotice message={notice} /><ErrorNotice message={detail.error} retry={detail.refresh} /><ErrorNotice message={action.error} />
    <section className="panel knowledge-detail-panel"><div className="knowledge-tabs" role="tablist" aria-label="知识库内容"><button role="tab" aria-selected={tab === 'documents'} onClick={() => setTab('documents')}>文档管理</button><button role="tab" aria-selected={tab === 'search'} onClick={() => setTab('search')}>检索测试</button></div>
      {tab === 'search' ? <RetrievalPanel api={api} base={base} onOpenDocument={documentId => { const document = documents.find(item => item.id === documentId); if (document) setDialog({ kind: 'chunks', document }) }} /> : <>
        <div className="knowledge-section-heading"><div><h2>文档 <span className="count-badge">{documents.length}</span></h2><p>文档处理完成后参与检索；处理状态自动刷新。</p></div><div className="toolbar-actions"><SearchForm placeholder="搜索文档标题" onSearch={setQuery} /><button className="icon-button" aria-label="刷新文档" onClick={detail.refresh}><Icon name="refresh" size={17} /></button></div></div>
        {!filtered.length ? <EmptyState title={query ? '没有匹配的文档' : '还没有文档'} description="上传资料或手动输入内容，开始构建知识库。" /> : <div className="table-scroll"><table><thead><tr><th>文档</th><th>处理状态</th><th>分块数</th><th>更新时间</th><th className="text-right">操作</th></tr></thead><tbody>{filtered.map(document => <tr key={document.id}>
          <td><button className="knowledge-document-title" onClick={() => setDialog({ kind: 'chunks', document })}>{document.title}</button><small className="knowledge-file-meta">{document.source === 'manual' ? '手动输入' : `${document.file_name} · ${(document.file_size / 1024).toFixed(1)} KiB`}</small></td>
          <td><span className={`badge ${document.parse_status === 'completed' ? 'green' : document.parse_status === 'failed' ? 'red' : 'gray'}`}>{statuses[document.parse_status]}</span>{document.error_message && <p className="knowledge-error">{document.error_message}</p>}</td><td>{document.chunk_count}</td><td>{new Date(document.updated_at).toLocaleString('zh-CN')}</td>
          <td><div className="row-actions"><button onClick={() => setDialog({ kind: 'chunks', document })}>查看分块</button>{document.source === 'manual' ? <button disabled={action.busy || document.parse_status === 'processing' || document.parse_status === 'queued' || document.parse_status === 'deleting'} onClick={() => setDialog({ kind: 'edit', document })}>编辑内容</button> : <button disabled={action.busy || document.parse_status === 'deleting'} onClick={() => { setNotice(''); void action.run(signal => api.downloadKnowledgeDocument(document.id, signal), blob => {
            const url = URL.createObjectURL(blob)
            const link = window.document.createElement('a'); link.href = url; link.download = document.file_name || document.title; link.click(); setTimeout(() => URL.revokeObjectURL(url), 0)
          }) }}>下载原件</button>}
            <button disabled={action.busy || document.parse_status === 'processing' || document.parse_status === 'queued' || document.parse_status === 'deleting'} onClick={() => { setNotice(''); void action.run(signal => api.reprocessKnowledgeDocument(document.id, signal), () => { setNotice('已加入处理队列，将从原文重新切分并覆盖分块编辑。'); detail.refresh() }) }}>{document.parse_status === 'failed' ? '重试' : '重新处理'}</button><button className="knowledge-danger" onClick={() => setDialog({ kind: 'delete-document', document })}>{document.parse_status === 'deleting' ? '重试删除' : '删除'}</button></div></td>
        </tr>)}</tbody></table></div>}
      </>}
    </section>
    <div className="knowledge-inline-actions"><button className="button secondary knowledge-danger" onClick={() => setDialog({ kind: 'delete-base' })}>删除知识库</button></div>
    {dialog?.kind === 'settings' && <KnowledgeForm api={api} base={base} onClose={() => setDialog(null)} onSaved={() => complete('知识库设置已保存。')} />}
    {(dialog?.kind === 'import' || dialog?.kind === 'edit') && <DocumentImport api={api} base={base} document={dialog.kind === 'edit' ? dialog.document : undefined} onClose={() => setDialog(null)} onSaved={() => complete('资料已保存，正在建立索引。')} />}
    {dialog?.kind === 'chunks' && <ChunkViewer api={api} document={documents.find(item => item.id === dialog.document.id) ?? dialog.document} onClose={() => setDialog(null)} onChanged={detail.refresh} />}
    {dialog?.kind === 'delete-document' && <ConfirmDelete title={`删除文档「${dialog.document.title}」？`} description="文档原件、分块及检索索引将被删除；处理中的文档会在后台任务退出后完成清理。" onClose={() => setDialog(null)} onDelete={signal => api.deleteKnowledgeDocument(dialog.document.id, signal)} onDeleted={() => complete('删除请求已受理，处理中的文档将自动完成清理。')} />}
    {dialog?.kind === 'delete-base' && <ConfirmDelete title={`删除知识库「${base.name}」？`} description="知识库内的全部文档、原件和检索索引将被删除。" onClose={() => setDialog(null)} onDelete={signal => api.deleteKnowledgeBase(base.id, signal)} onDeleted={() => navigate('/knowledge')} />}
  </div>
}
