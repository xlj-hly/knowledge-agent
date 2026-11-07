import { useId, useState } from 'react'
import { Button, Collapse, Drawer, Input, InputNumber, Select } from 'antd'
import type { Api } from '../../api/client'
import type { AISettings, AISettingsInput, Model, Presets } from '../../api/ai/types'
import type { VectorStore } from '../../api/knowledge/types'
import { ErrorNotice, Field } from '../../components/ui'
import { ChunkingPreview } from '../../components/ChunkingPreview'
import { useAction } from '../../hooks/async'
import { groupPatch, settingsGroups } from './settingsGroups'
import type { SettingsGroup } from './settingsGroups'

interface EditorProps {
  api: Api
  group: SettingsGroup
  settings: AISettings
  models: Model[]
  stores: VectorStore[]
  presets: Presets
  onSave: (patch: Partial<AISettingsInput>, signal: AbortSignal) => Promise<AISettings>
  onClose: () => void
  onSaved: (settings: AISettings) => void
}

export function SettingsEditor({ api, group, settings, models, stores, presets, onSave, onClose, onSaved }: EditorProps) {
  const [draft, setDraft] = useState(settings)
  const [validation, setValidation] = useState('')
  const action = useAction(group)
  const formId = useId()
  const definition = settingsGroups.find(item => item.id === group)
  const update = <K extends keyof AISettings>(key: K, value: AISettings[K]) => setDraft(current => ({ ...current, [key]: value }))
  const modelOptions = [{ value: '', label: '暂不设置' }, ...models.filter(model => model.kind === 'embedding').map(model => ({ value: model.id, label: `${model.name} · ${model.model_name}` }))]
  const numberField = (label: string, key: 'temperature' | 'max_tokens' | 'history_turns' | 'retrieval_limit' | 'similarity_threshold' | 'chunk_size' | 'chunk_overlap', min: number, max: number, step = 1, hint?: string) => <Field label={label} hint={hint}>
    <InputNumber aria-label={label} value={Number.isFinite(draft[key]) ? draft[key] : null} min={min} max={max} step={step} controls={false} onChange={value => update(key, value ?? Number.NaN)} />
  </Field>
  const submit = () => {
    const patch = groupPatch(group, draft)
    if (Object.values(patch).some(value => typeof value === 'number' && !Number.isFinite(value))) {
      setValidation('请填写有效的数值'); return
    }
    if (group === 'chunking' && draft.chunk_overlap >= draft.chunk_size) { setValidation('重叠长度必须小于分块长度'); return }
    setValidation('')
    void action.run(signal => onSave(patch, signal), onSaved)
  }
  return <Drawer open title={definition?.title} size={560} rootClassName="ai-settings-drawer" destroyOnHidden
    onClose={() => { if (!action.busy) onClose() }} closable={!action.busy} keyboard={!action.busy} mask={{ closable: !action.busy }}
    footer={<div className="ai-editor-actions"><Button aria-label="取消" disabled={action.busy} onClick={onClose}>取消</Button><Button aria-label="保存" type="primary" htmlType="submit" form={formId} loading={action.busy}>保存</Button></div>}>
    <p className="ai-editor-description">{definition?.description}</p>
    <form id={formId} className="ai-settings-editor" onSubmit={event => { event.preventDefault(); submit() }}>
      <fieldset disabled={action.busy}>
        {group === 'models' && <>
          <Field label="默认 Embedder"><Select virtual={false} aria-label="默认 Embedder" value={draft.embedding_model_id} options={modelOptions} onChange={value => update('embedding_model_id', value)} disabled={action.busy} /></Field>
          <Field label="默认向量存储"><Select virtual={false} aria-label="默认向量存储" value={draft.vector_store_id} options={[{ value: '', label: '暂不设置' }, ...stores.map(store => ({ value: store.id, label: store.name }))]} onChange={value => update('vector_store_id', value)} disabled={action.busy} /></Field>
          {!models.some(model => model.kind === 'embedding') && <p className="ai-editor-help">先添加 Embedder，再到此处选择知识库默认连接。</p>}
        </>}
        {group === 'prompt' && <>
          <Field label="提示词预设"><Select virtual={false} aria-label="提示词预设" placeholder="选择一个预设" options={presets.prompts.map(item => ({ value: item.id, label: item.name }))} onChange={value => { const preset = presets.prompts.find(item => item.id === value); if (preset) update('system_prompt', preset.content) }} disabled={action.busy} /></Field>
          <Field label="系统提示词"><Input.TextArea aria-label="系统提示词" rows={12} maxLength={16000} value={draft.system_prompt} onChange={event => update('system_prompt', event.target.value)} /></Field>
          <p className="ai-editor-help">正文可以直接修改，留空表示不添加企业系统提示词。</p>
        </>}
        {group === 'generation' && <>
          {numberField('温度 Temperature', 'temperature', 0, 2, 0.1, '数值越高，回答越灵活；实际效果取决于模型。')}
          {numberField('最大输出 Tokens', 'max_tokens', 1, 32768)}
          {numberField('携带历史轮数', 'history_turns', 0, 50)}
        </>}
        {group === 'retrieval' && <>
          <Field label="默认检索策略"><Select virtual={false} aria-label="默认检索策略" value={draft.retrieval_mode} options={[{ value: 'hybrid', label: '混合检索（推荐）' }, { value: 'vector', label: '语义向量' }, { value: 'keyword', label: '关键词' }]} onChange={value => update('retrieval_mode', value)} disabled={action.busy} /></Field>
          {numberField('检索返回数量', 'retrieval_limit', 1, 20)}
          {numberField('向量相似度阈值', 'similarity_threshold', 0, 1, 0.01, '仅纯向量检索生效；0 表示不额外过滤。')}
        </>}
        {group === 'chunking' && <>
          <Field label="切分预设"><Select virtual={false} aria-label="切分预设" placeholder="选择推荐方案" options={presets.chunking.map(item => ({ value: item.id, label: item.name }))} onChange={value => { const preset = presets.chunking.find(item => item.id === value); if (preset) setDraft(current => ({ ...current, chunk_strategy: preset.strategy, chunk_size: preset.size, chunk_overlap: preset.overlap })) }} disabled={action.busy} /></Field>
          <Field label="切分策略" hint={presets.chunk_strategies.find(item => item.id === draft.chunk_strategy)?.description}><Select virtual={false} aria-label="切分策略" value={draft.chunk_strategy} options={presets.chunk_strategies.map(item => ({ value: item.id, label: item.name }))} onChange={value => update('chunk_strategy', value)} disabled={action.busy} /></Field>
          <div className="ai-editor-number-grid">{numberField('分块长度', 'chunk_size', 64, 8192)}{numberField('重叠长度', 'chunk_overlap', 0, 8191)}</div>
          <ChunkingPreview api={api} strategy={draft.chunk_strategy} size={draft.chunk_size} overlap={draft.chunk_overlap} />
          <Collapse ghost items={[{ key: 'parser', label: 'PDF 解析服务', children: <Field label="默认 PDF 解析服务" hint="DocReader HTTP 地址。文本与 DOCX 使用内置解析。"><Input aria-label="默认 PDF 解析服务" type="url" value={draft.parser_endpoint} onChange={event => update('parser_endpoint', event.target.value)} placeholder="http://docreader:8080" /></Field> }]} />
        </>}
      </fieldset>
      <ErrorNotice message={validation || action.error} />
    </form>
  </Drawer>
}
