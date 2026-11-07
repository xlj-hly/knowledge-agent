import { useCallback, useState } from 'react'
import { Button } from 'antd'
import type { Api } from '../../api/client'
import type { AISettings, Model, Presets } from '../../api/ai/types'
import type { VectorStore } from '../../api/knowledge/types'
import { ErrorNotice, Icon, Loading, PageHeader, SuccessNotice } from '../../components/ui'
import { useResource } from '../../hooks/async'
import { SettingsEditor } from './SettingsEditor'
import { settingsGroups, retrievalName } from './settingsGroups'
import type { SettingsGroup } from './settingsGroups'
import './ai-settings.css'

export function AISettingsPage({ api }: { api: Api }) {
  const resource = useResource(useCallback(async (signal: AbortSignal) => {
    const [settings, models, stores, presets] = await Promise.all([
      api.aiSettings(signal), api.aiModels(signal), api.knowledgeVectorStores(signal), api.aiPresets(signal),
    ])
    return { settings, models, stores, presets }
  }, [api]))
  return <div className="ai-settings-page">
    <PageHeader eyebrow="AI PREFERENCES" title="默认配置与提示词" description="选择一项进行调整。更改用于新对话和新知识库。" />
    <ErrorNotice message={resource.error} retry={resource.refresh} />
    {resource.loading ? <Loading /> : resource.data && <SettingsOverview api={api} {...resource.data} />}
  </div>
}

function SettingsOverview({ api, settings, models, stores, presets }: {
  api: Api; settings: AISettings; models: Model[]; stores: VectorStore[]; presets: Presets
}) {
  const [current, setCurrent] = useState(settings)
  const [editing, setEditing] = useState<SettingsGroup | null>(null)
  const [notice, setNotice] = useState('')
  const modelName = (id: string) => models.find(model => model.id === id)?.name ?? '未选择'
  const summary = (group: SettingsGroup): string => {
    switch (group) {
      case 'models': return `Embedder：${modelName(current.embedding_model_id)} · 向量存储：${stores.find(store => store.id === current.vector_store_id)?.name ?? '未选择'}`
      case 'prompt': return current.system_prompt || '未设置系统提示词'
      case 'generation': return `温度 ${current.temperature} · 最多 ${current.max_tokens} Tokens · ${current.history_turns} 轮历史`
      case 'retrieval': return `${retrievalName(current.retrieval_mode)} · 返回 ${current.retrieval_limit} 个片段 · 向量阈值 ${current.similarity_threshold}`
      case 'chunking': return `${presets.chunk_strategies.find(item => item.id === current.chunk_strategy)?.name ?? current.chunk_strategy} · 每块 ${current.chunk_size} 字 · 重叠 ${current.chunk_overlap} 字${current.parser_endpoint ? ' · 已配置 PDF 解析' : ''}`
    }
  }
  return <>
    <SuccessNotice message={notice} />
    <section className="ai-settings-list" aria-label="默认配置项目">
      {settingsGroups.map(group => <article key={group.id} className="ai-setting-row">
        <span className="ai-setting-icon"><Icon name={group.icon} size={21} /></span>
        <div className="ai-setting-info"><h2>{group.title}</h2><p>{group.description}</p><div className="ai-setting-value">{summary(group.id)}</div></div>
        <Button aria-label={`编辑${group.title}`} onClick={() => { setNotice(''); setEditing(group.id) }}>编辑<Icon name="chevron" size={13} /></Button>
      </article>)}
    </section>
    <p className="ai-settings-footnote">LLM 在模型列表中启用，所有后续消息使用当前启用模型。此处模板用于新对话和新知识库，已有内容保留各自参数。</p>
    {editing && <SettingsEditor key={editing} api={api} group={editing} settings={current} models={models} stores={stores} presets={presets}
      onSave={(patch, signal) => api.saveAISettings({ ...current, ...patch }, signal)}
      onClose={() => setEditing(null)} onSaved={saved => { setCurrent(saved); setNotice(`${settingsGroups.find(group => group.id === editing)?.title}已保存`); setEditing(null) }} />}
  </>
}
