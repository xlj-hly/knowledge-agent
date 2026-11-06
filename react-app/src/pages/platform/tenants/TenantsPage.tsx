import { useCallback, useState } from 'react'
import { useResource } from '../../../hooks/async'
import {
  EmptyState,
  ErrorNotice,
  Icon,
  Loading,
  PageHeader,
  Pagination,
  SearchForm,
  StatusBadge,
  SuccessNotice,
} from '../../../components/ui'
import { TenantForm } from './components/TenantForm'
import { ResetAdminForm } from './components/ResetAdminForm'
import { WechatForm } from './components/WechatForm'
import type { TenantsApi } from '../../../api/tenants/api'
import type { Tenant } from '../../../api/tenants/types'
import './tenants.css'
type TenantDialog = { kind: 'create' } | { kind: 'edit' | 'reset' | 'wechat'; tenant: Tenant }

export function TenantsPage({ api }: { api: TenantsApi }) {
  const [q, setQ] = useState('')
  const [page, setPage] = useState(1)
  const [dialog, setDialog] = useState<TenantDialog | null>(null)
  const [success, setSuccess] = useState('')
  const load = useCallback((signal: AbortSignal) => api.tenants(q, page, signal), [api, q, page])
  const list = useResource(load)
  function complete(message: string) {
    setSuccess(message)
    setDialog(null)
    list.refresh()
  }
  return (
    <>
      <PageHeader
        eyebrow="ENTERPRISE MANAGEMENT"
        title="企业管理"
        description="连接每一家企业，管理独立的协作空间与访问权限。"
        action={
          <button className="button primary" onClick={() => setDialog({ kind: 'create' })}>
            <Icon name="plus" size={18} />
            开通企业
          </button>
        }
      />
      <div className="metric-grid">
        <Metric
          icon="building"
          title="企业空间"
          value={list.data ? String(list.data.total) : '—'}
          description={q ? '当前搜索结果' : '已开通的企业总数'}
        />
        <Metric
          icon="shield"
          title="独立的访问边界"
          value="专属域名"
          description="每个企业拥有独立的协作入口"
        />
        <Metric
          icon="users"
          title="安全的权限管理"
          value="分级管理"
          description="平台、企业、成员各司其职"
        />
      </div>
      <SuccessNotice message={success} />
      <section className="panel table-panel">
        <div className="table-toolbar">
          <div>
            <h2>
              企业列表 <span className="count-badge">{list.data?.total ?? '—'}</span>
            </h2>
            <p>开通与维护企业，配置访问状态。</p>
          </div>
          <div className="toolbar-actions">
            <SearchForm
              placeholder="搜索企业名称或域名"
              onSearch={(value) => {
                setQ(value)
                setPage(1)
              }}
            />
            <button
              className="icon-button bordered"
              title="刷新列表"
              aria-label="刷新列表"
              onClick={list.refresh}
            >
              <Icon name="refresh" size={17} />
            </button>
          </div>
        </div>
        {list.loading ? (
          <Loading label="正在加载企业列表…" />
        ) : list.error ? (
          <div className="table-message">
            <ErrorNotice message={list.error} retry={list.refresh} />
          </div>
        ) : (
          list.data && (
            <>
              <div className="table-scroll">
                <table>
                  <thead>
                    <tr>
                      <th>企业名称</th>
                      <th>访问域名</th>
                      <th>状态</th>
                      <th>创建时间</th>
                      <th className="text-right">操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {list.data.items.map((tenant) => (
                      <tr key={tenant.id}>
                        <td>
                          <div className="table-identity">
                            <span className="entity-icon">
                              <Icon name="building" size={19} />
                            </span>
                            <div>
                              <strong>{tenant.name}</strong>
                              <small>企业 ID · {tenant.id}</small>
                            </div>
                          </div>
                        </td>
                        <td>
                          <span className="hostname">
                            <Icon name="globe" size={14} />
                            {tenant.hostname}
                          </span>
                        </td>
                        <td>
                          <StatusBadge status={tenant.status} />
                        </td>
                        <td className="muted">
                          {new Date(tenant.created_at).toLocaleDateString('zh-CN')}
                        </td>
                        <td>
                          <div className="row-actions">
                            <button onClick={() => setDialog({ kind: 'edit', tenant })}>
                              编辑
                            </button>
                            <span />
                            <button onClick={() => setDialog({ kind: 'wechat', tenant })}>
                              小程序配置
                            </button>
                            <span />
                            <button onClick={() => setDialog({ kind: 'reset', tenant })}>
                              重置管理员密码
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {!list.data.items.length && (
                <EmptyState
                  title={q ? '未找到匹配的企业' : '从第一家企业开始'}
                  description={
                    q
                      ? '尝试使用其他企业名称或域名进行搜索。'
                      : '开通企业空间，同时创建第一位企业管理员。'
                  }
                />
              )}
              <Pagination
                page={page}
                total={list.data.total}
                pageSize={list.data.page_size}
                onChange={setPage}
              />
            </>
          )
        )}
      </section>
      <div className="bottom-tip">
        <Icon name="shield" size={17} />
        <span>停用企业后，该企业成员将无法访问，已有登录会话立即失效。</span>
      </div>
      {dialog?.kind === 'create' && (
        <TenantForm
          api={api}
          onClose={() => setDialog(null)}
          onSaved={(tenant) => complete(`「${tenant.name}」已开通，首位管理员账号已创建。`)}
        />
      )}
      {dialog?.kind === 'edit' && (
        <TenantForm
          api={api}
          tenant={dialog.tenant}
          onClose={() => setDialog(null)}
          onSaved={(tenant) => complete(`「${tenant.name}」的企业信息已更新。`)}
        />
      )}
      {dialog?.kind === 'reset' && (
        <ResetAdminForm
          api={api}
          tenant={dialog.tenant}
          onClose={() => setDialog(null)}
          onSaved={() => complete('管理员密码已重置，其所有登录会话已失效。')}
        />
      )}
      {dialog?.kind === 'wechat' && (
        <WechatForm key={dialog.tenant.id} api={api} tenant={dialog.tenant} onClose={() => setDialog(null)} onSaved={() => complete(`「${dialog.tenant.name}」的小程序配置已保存。`)} />
      )}
    </>
  )
}

function Metric({
  icon,
  title,
  value,
  description,
}: {
  icon: Parameters<typeof Icon>[0]['name']
  title: string
  value: string
  description: string
}) {
  return (
    <section className="metric-card">
      <div className="metric-top">
        <span>{title}</span>
        <span className="metric-icon">
          <Icon name={icon} size={19} />
        </span>
      </div>
      <strong>{value}</strong>
      <p>{description}</p>
    </section>
  )
}
