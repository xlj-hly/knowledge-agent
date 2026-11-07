import { useCallback, useState } from 'react'
import { Link } from 'react-router'
import type { Api } from '../../../api/client'
import type { AISettings, Model, Presets } from '../../../api/ai/types'
import type { IndexingMode, KnowledgeBase, KnowledgeBaseInput, VectorStore } from '../../../api/knowledge/types'
import { ErrorNotice, Field, Loading, Modal } from '../../../components/ui'
import { ChunkingPreview } from '../../../components/ChunkingPreview'
import { useAction, useResource } from '../../../hooks/async'
import { modeLabels } from '../labels'

type Props = { api: Api; base?: KnowledgeBase; onClose: () => void; onSaved: (base: KnowledgeBase) => void }
export function KnowledgeForm(props: Props) {
  const options = useResource(useCallback(async (signal: AbortSignal) => {
    const [models, stores, settings, presets] = await Promise.all([props.api.aiModels(signal), props.api.knowledgeVectorStores(signal), props.api.aiSettings(signal), props.api.aiPresets(signal)])
    return { models: models.filter(item => item.kind === 'embedding'), stores, settings, presets }
  }, [props.api]))
  if (!options.data) return <Modal title={props.base ? '知识库设置' : '新建知识库'} description="正在读取可用模型、连接与默认值。" onClose={props.onClose}>{options.loading ? <Loading label="正在读取企业默认配置…" /> : <ErrorNotice message={options.error} retry={options.refresh} />}</Modal>
  return <Form {...props} {...options.data} />
}

function Form({ api, base, onClose, onSaved, models, stores, settings, presets }: Props & { models: Model[]; stores: VectorStore[]; settings: AISettings; presets: Presets }) {
  const [draft, setDraft] = useState<KnowledgeBaseInput>(() => base ? { ...base } : { name: '', description: '', indexing_mode: 'hybrid', retrieval_mode: settings.retrieval_mode, embedding_model_id: settings.embedding_model_id, vector_store_id: settings.vector_store_id, chunk_strategy: settings.chunk_strategy, chunk_size: settings.chunk_size, chunk_overlap: settings.chunk_overlap, parser_endpoint: settings.parser_endpoint })
  const [confirmed, setConfirmed] = useState(false)
  const [validation, setValidation] = useState('')
  const action = useAction(base?.id)
  const update = <K extends keyof KnowledgeBaseInput>(key: K, value: KnowledgeBaseInput[K]) => { setDraft(current => ({ ...current, [key]: value })); setConfirmed(false) }
  const needsRebuild = Boolean(base?.document_count && (draft.indexing_mode !== base.indexing_mode || draft.embedding_model_id !== base.embedding_model_id || draft.vector_store_id !== base.vector_store_id || draft.chunk_strategy !== base.chunk_strategy || draft.chunk_size !== base.chunk_size || draft.chunk_overlap !== base.chunk_overlap || draft.parser_endpoint !== base.parser_endpoint))
  return <Modal title={base ? '知识库设置' : '新建知识库'} description={base ? '查询策略可直接调整；影响资料处理的配置通过重建应用。' : '默认使用企业的嵌入器、向量连接和文本分割配置。'} onClose={onClose} busy={action.busy}>
    <form className="stacked-form modal-form knowledge-form" noValidate onSubmit={event => {
      event.preventDefault()
      if (!draft.name.trim()) { setValidation('请输入知识库名称'); return }
      if (draft.indexing_mode !== 'keyword' && (!draft.embedding_model_id || !draft.vector_store_id)) { setValidation('请企业管理员先配置默认 Embedder 和向量存储，或在高级配置中选择连接'); return }
      if (needsRebuild && !confirmed) { setValidation('请确认重新处理当前知识库的全部资料'); return }
      if (!Number.isInteger(draft.chunk_size) || draft.chunk_size < 64 || draft.chunk_size > 8192 || !Number.isInteger(draft.chunk_overlap) || draft.chunk_overlap < 0 || draft.chunk_overlap >= draft.chunk_size) { setValidation('分块长度须为64–8192，重叠长度须小于分块长度'); return }
      setValidation('')
      const input = { ...draft, name: draft.name.trim(), description: draft.description.trim(), embedding_model_id: draft.indexing_mode === 'keyword' ? '' : draft.embedding_model_id, vector_store_id: draft.indexing_mode === 'keyword' ? '' : draft.vector_store_id }
      void action.run(signal => base && needsRebuild ? api.rebuildKnowledgeBase(base.id, input, signal) : api.saveKnowledgeBase(input, base?.id, signal), onSaved)
    }}>
      <fieldset disabled={action.busy}>
        <Field label="知识库名称"><input aria-label="知识库名称" value={draft.name} maxLength={100} onChange={event => update('name', event.target.value)} /></Field>
        <Field label="描述"><textarea aria-label="描述" rows={3} value={draft.description} maxLength={1000} onChange={event => update('description', event.target.value)} placeholder="这个知识库包含哪些资料？" /></Field>
        {!base && <p className="knowledge-hint">混合检索同时查找关键词与相近语义。当前配置：{models.find(model => model.id === draft.embedding_model_id)?.name ?? '尚未选择 Embedder'} · {stores.find(store => store.id === draft.vector_store_id)?.name ?? '尚未选择向量存储'}。<Link to="/ai-settings">管理企业默认值</Link></p>}
        <details className="knowledge-advanced" open={base ? true : undefined}><summary>高级配置</summary>
          <Field label="索引能力" hint="决定文档处理时准备哪些索引。"><select aria-label="索引能力" value={draft.indexing_mode} onChange={event => { const value = event.target.value; if (value === 'keyword' || value === 'hybrid') { update('indexing_mode', value); if (value === 'keyword') update('retrieval_mode', 'keyword') } }}><option value="hybrid">关键词与向量（推荐）</option><option value="keyword">仅关键词</option></select></Field>
          <Field label="默认检索策略" hint="两种索引都可用时，切换查询策略无需重建。"><select aria-label="默认检索策略" value={draft.retrieval_mode} onChange={event => { const value = event.target.value; if (value === 'keyword' || value === 'vector' || value === 'hybrid') update('retrieval_mode', value) }}>{(draft.indexing_mode === 'keyword' ? ['keyword'] : ['hybrid', 'vector', 'keyword']).map(value => <option key={value} value={value}>{modeLabels[value as IndexingMode]}</option>)}</select></Field>
          {draft.indexing_mode !== 'keyword' && <><Field label="Embedding 模型"><select aria-label="Embedding 模型" value={draft.embedding_model_id} onChange={event => update('embedding_model_id', event.target.value)}><option value="">请选择嵌入器</option>{models.map(item => <option key={item.id} value={item.id}>{item.name} · {item.model_name}</option>)}</select></Field><Field label="向量存储"><select aria-label="向量存储" value={draft.vector_store_id} onChange={event => update('vector_store_id', event.target.value)}><option value="">请选择向量存储</option>{stores.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></Field></>}
          <Field label="切分预设"><select aria-label="切分预设" value="" onChange={event => { const preset = presets.chunking.find(item => item.id === event.target.value); if (preset) { setDraft(current => ({ ...current, chunk_strategy: preset.strategy, chunk_size: preset.size, chunk_overlap: preset.overlap })); setConfirmed(false) } }}><option value="">选择推荐方案</option>{presets.chunking.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></Field>
          <Field label="切分策略" hint={presets.chunk_strategies.find(item => item.id === draft.chunk_strategy)?.description}><select aria-label="切分策略" value={draft.chunk_strategy} onChange={event => update('chunk_strategy', event.target.value)}>{presets.chunk_strategies.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></Field>
          <div className="knowledge-form-row"><Field label="分块长度" hint="字符数"><input aria-label="分块长度" type="number" min={64} max={8192} value={draft.chunk_size} onChange={event => update('chunk_size', event.target.valueAsNumber)} /></Field><Field label="重叠长度"><input aria-label="重叠长度" type="number" min={0} max={8191} value={draft.chunk_overlap} onChange={event => update('chunk_overlap', event.target.valueAsNumber)} /></Field></div>
          <ChunkingPreview api={api} strategy={draft.chunk_strategy} size={draft.chunk_size} overlap={draft.chunk_overlap} />
          <Field label="PDF 解析服务地址" hint="DocReader HTTP 地址；文本和 DOCX 使用内置解析。"><input aria-label="PDF 解析服务地址" type="url" value={draft.parser_endpoint} onChange={event => update('parser_endpoint', event.target.value)} placeholder="http://docreader:8080" /></Field>
        </details>
        {needsRebuild && <div className="knowledge-rebuild-notice"><p>保存后将重新处理 {base?.document_count} 篇资料。处理期间未完成的资料暂不参与检索，原件保留，原有分块编辑会被重新生成的结果替换。</p><label><input type="checkbox" checked={confirmed} onChange={event => setConfirmed(event.target.checked)} />确认重建当前知识库的全部资料</label>{Boolean(base?.processing_count) && <p>当前还有 {base?.processing_count} 篇资料正在处理，请等待完成后重建。</p>}</div>}
      </fieldset>
      <ErrorNotice message={validation || action.error} />
      <div className="modal-footer"><button type="button" className="button secondary" disabled={action.busy} onClick={onClose}>取消</button><button className="button primary" disabled={action.busy || needsRebuild && Boolean(base?.processing_count)}>{action.busy ? '正在保存…' : needsRebuild ? '保存并重建全部文档' : base ? '保存设置' : '创建知识库'}</button></div>
    </form>
  </Modal>
}
