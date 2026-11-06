import { useCallback, useState } from 'react'
import { useResource } from '../../../hooks/async'
import {
  Avatar,
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
import { MemberForm } from './components/MemberForm'
import { ResetUserPassword } from './components/ResetUserPassword'
import type { MembersApi } from '../../../api/members'
import type { User } from '../../../api/account/types'
import './members.css'
type MemberDialog = { kind: 'create' } | { kind: 'edit' | 'reset'; user: User }

export function MembersPage({
  api,
  currentUser,
  onSelfChanged,
}: {
  api: MembersApi
  currentUser: User
  onSelfChanged: (user: User) => void
}) {
  const [q, setQ] = useState('')
  const [page, setPage] = useState(1)
  const [dialog, setDialog] = useState<MemberDialog | null>(null)
  const [success, setSuccess] = useState('')
  const load = useCallback((signal: AbortSignal) => api.users(q, page, signal), [api, q, page])
  const list = useResource(load)
  function complete(message: string, user?: User) {
    setSuccess(message)
    setDialog(null)
    list.refresh()
    if (
      user?.id === currentUser.id &&
      (user.role !== currentUser.role || user.status !== currentUser.status)
    )
      onSelfChanged(user)
  }
  return (
    <>
      <PageHeader
        eyebrow="TEAM MEMBERS"
        title="成员管理"
        description="让合适的人拥有合适的权限，建立有序的团队。"
        action={
          <button className="button primary" onClick={() => setDialog({ kind: 'create' })}>
            <Icon name="plus" size={18} />
            添加成员
          </button>
        }
      />
      <div className="members-summary">
        <span className="section-icon">
          <Icon name="users" size={24} />
        </span>
        <div>
          <strong>
            {list.data?.total ?? '—'} <small>位团队成员</small>
          </strong>
          <p>成员各司其职，协作井然有序。</p>
        </div>
        <div className="summary-hint">
          <Icon name="shield" size={17} />
          企业管理员可管理成员与企业设置
        </div>
      </div>
      <SuccessNotice message={success} />
      <section className="panel table-panel">
        <div className="table-toolbar">
          <div>
            <h2>
              团队成员 <span className="count-badge">{list.data?.total ?? '—'}</span>
            </h2>
            <p>管理成员的账号、角色与访问权限。</p>
          </div>
          <div className="toolbar-actions">
            <SearchForm
              placeholder="搜索用户名或昵称"
              onSearch={(value) => {
                setQ(value)
                setPage(1)
              }}
            />
            <button
              className="icon-button bordered"
              aria-label="刷新列表"
              title="刷新列表"
              onClick={list.refresh}
            >
              <Icon name="refresh" size={17} />
            </button>
          </div>
        </div>
        {list.loading ? (
          <Loading label="正在加载团队成员…" />
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
                      <th>团队成员</th>
                      <th>用户名</th>
                      <th>角色</th>
                      <th>状态</th>
                      <th>加入时间</th>
                      <th className="text-right">操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {list.data.items.map((user) => (
                      <tr key={user.id}>
                        <td>
                          <div className="table-identity">
                            <Avatar name={user.nickname || user.username} url={user.avatar_url} />
                            <div>
                              <strong>
                                {user.nickname || user.username}
                                {user.id === currentUser.id && (
                                  <span className="self-label">你</span>
                                )}
                              </strong>
                              <small>成员 ID · {user.id}</small>
                            </div>
                          </div>
                        </td>
                        <td className="mono">{user.username}</td>
                        <td>
                          <span className={`badge ${user.role === 'admin' ? 'purple' : 'gray'}`}>
                            {user.role === 'admin' ? '企业管理员' : '普通成员'}
                          </span>
                        </td>
                        <td>
                          <StatusBadge status={user.status} />
                        </td>
                        <td className="muted">
                          {new Date(user.created_at).toLocaleDateString('zh-CN')}
                        </td>
                        <td>
                          <div className="row-actions">
                            <button onClick={() => setDialog({ kind: 'edit', user })}>
                              编辑权限
                            </button>
                            <span />
                            <button onClick={() => setDialog({ kind: 'reset', user })}>
                              重置密码
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
                  title="未找到匹配的成员"
                  description="尝试其他用户名或昵称，或添加新的团队成员。"
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
        <span>企业至少需要一位正常状态的管理员；停用或重置密码后，成员已有的登录会话将失效。</span>
      </div>
      {dialog?.kind === 'create' && (
        <MemberForm
          api={api}
          onClose={() => setDialog(null)}
          onSaved={(user) => complete(`成员「${user.nickname || user.username}」已添加。`, user)}
        />
      )}
      {dialog?.kind === 'edit' && (
        <MemberForm
          api={api}
          user={dialog.user}
          onClose={() => setDialog(null)}
          onSaved={(user) => complete(`「${user.nickname || user.username}」的权限已更新。`, user)}
        />
      )}
      {dialog?.kind === 'reset' && (
        <ResetUserPassword
          api={api}
          user={dialog.user}
          onClose={() => setDialog(null)}
          onSaved={() => {
            if (dialog.user.id === currentUser.id) onSelfChanged(dialog.user)
            else complete('密码已重置，该成员需要使用新密码重新登录。')
          }}
        />
      )}
    </>
  )
}
