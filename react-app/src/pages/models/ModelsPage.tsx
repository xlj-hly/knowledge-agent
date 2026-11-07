import { useCallback, useState } from 'react'
import { Button, Input, Tag } from 'antd'
import type { Api } from '../../api/client'
import type { Model, ModelKind } from '../../api/ai/types'
import { EmptyState, ErrorNotice, Icon, Loading, PageHeader, SuccessNotice } from '../../components/ui'
import { ConfirmDelete } from '../../components/ui/ConfirmDelete'
import { useAction, useResource } from '../../hooks/async'
import { ModelForm } from './components/ModelForm'
import { ProviderLogo } from '../../components/ProviderLogo'
import './models.css'

export function ModelsPage({ api, kind = 'chat' }: { api: Api; kind?: ModelKind }) {
  const resource = useResource(useCallback(async (signal: AbortSignal) => {
    const [models, providers, settings] = await Promise.all([api.aiModels(signal), api.aiProviders(signal), api.aiSettings(signal)])
    return { models: models.filter(model => model.kind === kind), providers, settings }
  }, [api, kind]))
  const [editing, setEditing] = useState<Model | 'new' | null>(null)
  const [initialProvider, setInitialProvider] = useState<string>()
  const [removing, setRemoving] = useState<Model | null>(null)
  const [notice, setNotice] = useState('')
  const [query, setQuery] = useState('')
  const [activating, setActivating] = useState('')
  const activation = useAction(kind)
  const activate = (item: Model) => {
    setNotice(''); setActivating(item.id)
    void activation.run(signal => api.activateAIModel(item.id, signal), () => {
      setNotice(`已启用「${item.name}」，后续消息将使用此模型。`)
      resource.refresh()
    })
  }
  const visibleProviders = resource.data?.providers.filter(provider => provider.kinds.includes(kind) && `${provider.name} ${provider.description}`.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase())) ?? []
  const title = kind === 'chat' ? 'LLM 对话模型' : '嵌入器 Embedder'
  return <div className="models-page">
    <PageHeader eyebrow={kind === 'chat' ? 'LANGUAGE MODELS' : 'EMBEDDING MODELS'} title={title}
      description={kind === 'chat' ? '启用一个模型，供当前企业的所有对话使用。' : '配置将资料和问题转换为向量的模型，供知识库建立语义索引。'}
      action={<Button type="primary" icon={<Icon name="plus" size={17} />} onClick={() => { setInitialProvider(undefined); setEditing('new') }}>添加模型</Button>} />
    <SuccessNotice message={notice} /><ErrorNotice message={resource.error} retry={resource.refresh} /><ErrorNotice message={activation.error} />
    {resource.loading ? <Loading /> : resource.data && <>
      <section className="provider-section" aria-label="支持的服务商"><div className="provider-section-heading"><div><h2>选择服务商</h2><p>点击服务商，直接配置模型连接。</p></div><Input aria-label="搜索服务商" placeholder="搜索名称或描述" prefix={<Icon name="search" size={15} />} value={query} allowClear onChange={event => setQuery(event.target.value)} /></div><div className="provider-catalog">{visibleProviders.map(provider => <button key={provider.id} className="provider-card" aria-label={`配置 ${provider.name}`} onClick={() => { setInitialProvider(provider.id); setEditing('new') }}><ProviderLogo logo={provider.logo} size={32} /><span><strong>{provider.name}</strong><small>{provider.description}</small></span><Icon name="chevron" size={15} /></button>)}</div>{!visibleProviders.length && <p className="provider-empty" role="status">没有匹配的服务商，请调整搜索内容。</p>}</section>
      <section className="panel table-panel"><div className="table-toolbar"><div><h2>已配置模型 <span className="count-badge">{resource.data.models.length}</span></h2><p>配置属于当前企业，可随时编辑和测试连接。</p></div><button className="icon-button" aria-label="刷新模型" onClick={resource.refresh}><Icon name="refresh" size={17} /></button></div>
        {!resource.data.models.length ? <EmptyState title="尚未配置模型" description="选择上方服务商，添加第一个模型。" /> : <div className="table-scroll"><table><thead><tr><th>名称 / 模型</th><th>服务商</th><th>服务地址</th>{kind === 'embedding' && <th>维度</th>}<th>API Key</th><th className="text-right">操作</th></tr></thead><tbody>{resource.data.models.map(item => <tr key={item.id}><td><div className="table-identity"><ProviderLogo logo={resource.data?.providers.find(provider => provider.id === item.provider)?.logo} size={28} /><div><strong>{item.name}</strong><small>{item.model_name}</small></div>{kind === 'chat' && item.id === resource.data?.settings.chat_model_id && <Tag color="green">已启用</Tag>}</div></td><td>{resource.data?.providers.find(provider => provider.id === item.provider)?.name ?? item.provider}</td><td className="model-endpoint">{item.endpoint}</td>{kind === 'embedding' && <td>{item.dimension}</td>}<td><Tag>{item.api_key ? '已配置' : '未配置'}</Tag></td><td><div className="row-actions">{kind === 'chat' && item.id !== resource.data?.settings.chat_model_id && <Button aria-label="启用" type="link" size="small" disabled={activation.busy} loading={activation.busy && activating === item.id} onClick={() => activate(item)}>启用</Button>}<button disabled={activation.busy} onClick={() => setEditing(item)}>编辑与测试</button><span /><button disabled={activation.busy} onClick={() => setRemoving(item)}>删除</button></div></td></tr>)}</tbody></table></div>}
      </section>
      <p className="connection-tip">{kind === 'chat' ? '切换启用模型对新对话和已有对话的下一条消息生效，正在生成的回答会继续完成。' : '在「默认配置」中选择新知识库使用的 Embedder。已有知识库更换模型需重建索引。'}</p>
      {editing && <ModelForm api={api} kind={kind} providers={resource.data.providers} initialProvider={initialProvider} model={editing === 'new' ? undefined : editing} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); setNotice('模型配置已保存。'); resource.refresh() }} />}
    </>}
    {removing && <ConfirmDelete title={`删除模型「${removing.name}」？`} description="正在被知识库或企业配置引用的模型不能删除。" onDelete={signal => api.deleteAIModel(removing.id, signal)} onClose={() => setRemoving(null)} onDeleted={() => { setRemoving(null); resource.refresh(); setNotice('模型已删除。') }} />}
  </div>
}
