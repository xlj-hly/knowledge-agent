import type { PlatformAdmin } from '../../api/auth/types'
import type { User } from '../../api/account/types'
import type { Tenant } from '../../api/tenants/types'
import { pagePath } from '../../router/navigation'
import { Link } from 'react-router'
import { PageHeader, Icon, StatusBadge } from '../../components/ui'
import './overview.css'
export function OverviewPage({
  admin,
  user,
  tenant,
  platform,
}: {
  admin: PlatformAdmin | null
  user: User | null
  tenant: Tenant | null
  platform: boolean
}) {
  const name = user?.nickname || user?.username || admin?.username || ''
  return (
    <>
      <PageHeader
        eyebrow={admin ? 'PLATFORM OVERVIEW' : 'WORKSPACE OVERVIEW'}
        title={`你好，${name}`}
        description={
          admin
            ? '在这里连接企业，管理每一个独立的协作空间。'
            : '新的一天，从井然有序的工作空间开始。'
        }
      />
      <section className="welcome-banner">
        <div>
          <span className="welcome-label">
            <Icon name="spark" size={16} />
            {admin ? '平台管理中心' : tenant?.name}
          </span>
          <h2>{admin ? '每一家企业，都值得被妥善连接。' : '团队协作，从清晰的连接开始。'}</h2>
          <p>
            {admin
              ? '统一开通企业空间，配置域名与访问权限，让管理更从容。'
              : user?.role === 'admin'
                ? '管理成员、维护企业信息，让团队在安全的空间中共同成长。'
                : '管理个人资料与登录安全，让每一次连接都更加安心。'}
          </p>
          <Link
            className="button banner-button"
            to={pagePath(platform, admin ? 'tenants' : user?.role === 'admin' ? 'members' : 'account')}
          >
            {admin ? '管理企业空间' : user?.role === 'admin' ? '管理团队成员' : '管理个人账号'}
            <Icon name="arrow" size={17} />
          </Link>
        </div>
        <div className="banner-illustration" aria-hidden="true">
          <div className="banner-ring" />
          <span className="banner-building">
            <Icon name={admin ? 'building' : 'users'} size={62} />
          </span>
          <span className="banner-shield">
            <Icon name="shield" size={23} />
          </span>
          <span className="banner-dot" />
        </div>
      </section>
      <div className="overview-grid">
        <section className="panel">
          <div className="panel-title">
            <h2>{admin ? '平台身份' : '当前空间'}</h2>
            <span className="badge purple">{admin ? '平台管理' : '企业协作'}</span>
          </div>
          <dl className="details">
            <div>
              <dt>{admin ? '管理员账号' : '企业名称'}</dt>
              <dd>{admin ? admin.username : tenant?.name}</dd>
            </div>
            {tenant && (
              <>
                <div>
                  <dt>访问域名</dt>
                  <dd className="mono">{tenant.hostname}</dd>
                </div>
                <div>
                  <dt>空间状态</dt>
                  <dd>
                    <StatusBadge status={tenant.status} />
                  </dd>
                </div>
                <div>
                  <dt>自主注册</dt>
                  <dd>{tenant.registration_enabled ? '已开放' : '已关闭'}</dd>
                </div>
              </>
            )}
            <div>
              <dt>你的角色</dt>
              <dd>{admin ? '平台管理员' : user?.role === 'admin' ? '企业管理员' : '企业成员'}</dd>
            </div>
          </dl>
        </section>
        <section className="panel">
          <div className="panel-title">
            <h2>快速访问</h2>
            <Icon name="arrow" size={18} />
          </div>
          <div className="quick-links">
            {admin && (
              <QuickLink
                icon="building"
                title="企业管理"
                description="开通企业、配置域名与访问状态"
                to={pagePath(platform, 'tenants')}
              />
            )}
            {user?.role === 'admin' && (
              <>
                <QuickLink
                  icon="users"
                  title="成员管理"
                  description="邀请新成员，维护团队角色与权限"
                  to={pagePath(platform, 'members')}
                />
                <QuickLink
                  icon="settings"
                  title="企业设置"
                  description="更新企业信息与自主注册设置"
                  to={pagePath(platform, 'settings')}
                />
              </>
            )}
            <QuickLink
              icon="user"
              title="个人账号"
              description="管理个人资料，更新登录密码"
              to={pagePath(platform, 'account')}
            />
          </div>
        </section>
      </div>
    </>
  )
}
function QuickLink({
  icon,
  title,
  description,
  to,
}: {
  icon: Parameters<typeof Icon>[0]['name']
  title: string
  description: string
  to: string
}) {
  return (
    <Link className="quick-link" to={to}>
      <span className="quick-icon">
        <Icon name={icon} />
      </span>
      <span>
        <strong>{title}</strong>
        <small>{description}</small>
      </span>
      <Icon name="chevron" size={16} />
    </Link>
  )
}
