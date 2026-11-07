import { useCallback, useState } from 'react'
import { Button, Input, Tag } from 'antd'
import type { Api } from '../../api/client'
import type { VectorProvider, VectorStore } from '../../api/knowledge/types'
import { EmptyState, ErrorNotice, Icon, Loading, PageHeader, SuccessNotice } from '../../components/ui'
import { ProviderLogo } from '../../components/ProviderLogo'
import { ConfirmDelete } from '../../components/ui/ConfirmDelete'
import { useAction, useResource } from '../../hooks/async'
import { VectorStoreForm } from './components/VectorStoreForm'
import './vector-stores.css'

export function VectorStoresPage({ api }: { api: Api }) {
  const list = useResource(useCallback(async (signal: AbortSignal) => {
    const [stores, providers] = await Promise.all([api.knowledgeVectorStores(signal), api.knowledgeVectorProviders(signal)])
    return { stores, providers }
  }, [api]))
  const action = useAction()
  const [editing, setEditing] = useState<{ store?: VectorStore; provider: VectorProvider } | null>(null)
  const [removing, setRemoving] = useState<VectorStore | null>(null)
  const [notice, setNotice] = useState('')
  const [search, setSearch] = useState('')
  const providers = list.data?.providers.filter(provider => `${provider.name} ${provider.id} ${provider.description}`.toLowerCase().includes(search.trim().toLowerCase())) ?? []
  return <div className="vector-stores-page">
    <PageHeader eyebrow="VECTOR STORES" title="向量存储" description="选择引擎，配置知识库使用的向量连接。" />
    <SuccessNotice message={notice} /><ErrorNotice message={action.error} /><ErrorNotice message={list.error} retry={list.refresh} />
    <section aria-label="支持的向量引擎">
      <div className="vector-catalog-toolbar"><h2>添加连接</h2><Input.Search aria-label="搜索向量引擎" placeholder="搜索向量引擎" allowClear value={search} onChange={event => setSearch(event.target.value)} /></div>
      <div className="vector-engine-grid">{providers.map(provider => <button key={provider.id} className="vector-engine" onClick={() => setEditing({ provider })} aria-label={`添加 ${provider.name} 连接`}>
        <ProviderLogo logo={provider.logo} size={36} /><div><h3>{provider.name}</h3><p>{provider.description}</p></div><Icon name="plus" size={17} />
      </button>)}</div>
      {list.data && !providers.length && <EmptyState title="没有匹配的向量引擎" description="尝试其他名称。" />}
    </section>
    <section className="panel table-panel"><div className="table-toolbar"><h2>已配置连接 <span className="count-badge">{list.data?.stores.length ?? 0}</span></h2><Button type="text" aria-label="刷新向量存储" onClick={list.refresh} icon={<Icon name="refresh" size={17} />} /></div>
      {list.loading ? <Loading /> : list.data && (!list.data.stores.length ? <EmptyState title="尚未配置向量存储" description="选择上方引擎添加连接，即可在知识库中使用。" /> : <div className="table-scroll"><table>
        <thead><tr><th>名称</th><th>引擎</th><th>连接参数</th><th>凭据</th><th className="text-right">操作</th></tr></thead>
        <tbody>{list.data.stores.map(item => {
          const provider = list.data?.providers.find(provider => provider.id === item.provider)
          return <tr key={item.id}><td><div className="vector-connection-name"><ProviderLogo logo={provider?.logo} size={26} /><strong>{item.name}</strong>{item.referenced && <Tag>使用中</Tag>}</div></td><td>{provider?.name ?? item.provider}</td>
            <td className="vector-endpoint">{provider?.fields.filter(field => !field.secret && field.immutable && item.config[field.key]).map(field => <div key={field.key}>{field.label}：{item.config[field.key]}</div>)}</td>
            <td><Tag>{provider?.fields.some(field => field.secret && Boolean(item.config[field.key])) ? '已配置' : '未配置'}</Tag></td>
            <td><div className="row-actions"><button disabled={action.busy} onClick={() => { setNotice(''); void action.run(signal => api.testKnowledgeVectorStore(item.id, signal), () => setNotice(`「${item.name}」连接测试通过。`)) }}>测试连接</button><span /><button disabled={!provider} onClick={() => { if (provider) setEditing({ store: item, provider }) }}>编辑</button><span /><button disabled={item.referenced} onClick={() => setRemoving(item)}>删除</button></div></td></tr>
        })}</tbody>
      </table></div>)}
    </section>
    <p className="vector-tip">使用中的连接固定索引归属参数，凭据仍可更新。更换引擎请添加新连接，并在知识库中重建索引。</p>
    {editing && <VectorStoreForm api={api} {...editing} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); setNotice('向量存储配置已保存。'); list.refresh() }} />}
    {removing && <ConfirmDelete title={`删除连接「${removing.name}」？`} description="被知识库引用的连接无法删除。" onDelete={signal => api.deleteKnowledgeVectorStore(removing.id, signal)} onClose={() => setRemoving(null)} onDeleted={() => { setRemoving(null); list.refresh(); setNotice('连接已删除。') }} />}
  </div>
}
