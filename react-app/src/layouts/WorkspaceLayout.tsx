import type { ReactNode } from 'react'
import type { Identity } from '../stores/session/types'
import type { NavItemProps } from './NavItem'
import { Link } from 'react-router'
import { Avatar, Brand, Icon } from '../components/ui'
import { NavItem } from './NavItem'
import './layout.css'

export function WorkspaceLayout({
  identity,
  platform,
  homePath,
  accountPath,
  navigation,
  title,
  onSignOut,
  signingOut,
  children,
}: {
  identity: Identity
  platform: boolean
  homePath: string
  accountPath: string
  navigation: NavItemProps[]
  title: string
  onSignOut: () => void
  signingOut: boolean
  children: ReactNode
}) {
  const admin = 'admin' in identity ? identity.admin : null
  const user = 'user' in identity ? identity.user : null
  const tenant = 'tenant' in identity ? identity.tenant : null
  const displayName = user?.nickname || user?.username || admin?.username || ''
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <Link className="brand-link" to={homePath}>
          <Brand platform={platform} />
        </Link>
        <div className="workspace-label">
          {platform ? '平台管理空间' : tenant?.name}
          <span>{platform ? 'PLATFORM' : 'ENTERPRISE'}</span>
        </div>
        <nav aria-label="主导航">
          <span className="nav-group-label">工作空间</span>
          {navigation.map(item => <NavItem key={item.to} {...item} />)}
          <span className="nav-group-label second">账号</span>
          <NavItem
            icon="user"
            label="个人账号"
            to={accountPath}
          />
        </nav>
        <div className="sidebar-bottom">
          <div className="secure-note">
            <Icon name="shield" />
            <div>
              <strong>安心协作，从这里开始</strong>
              <span>企业独立空间 · 权限有序管理</span>
            </div>
          </div>
          <div className="sidebar-profile">
            <Avatar name={displayName} url={user?.avatar_url} />
            <div>
              <strong>{displayName}</strong>
              <small>
                {admin ? '平台管理员' : user?.role === 'admin' ? '企业管理员' : '企业成员'}
              </small>
            </div>
            <button
              className="icon-button"
              title="退出登录"
              aria-label="退出登录"
              disabled={signingOut}
              onClick={onSignOut}
            >
              <Icon name="logout" size={18} />
            </button>
          </div>
        </div>
      </aside>
      <div className="main-shell">
        <header className="topbar">
          <div className="breadcrumb">
            <span>{platform ? '管理控制台' : tenant?.name}</span>
            <Icon name="chevron" size={13} />
            <strong>{title}</strong>
          </div>
          <div className="topbar-right">
            <span className="environment-label">
              <i />
              {platform ? '平台空间' : '企业空间'}
            </span>
            <Link
              className="avatar-button"
              aria-label="打开个人账号"
              to={accountPath}
            >
              <Avatar name={displayName} url={user?.avatar_url} />
            </Link>
          </div>
        </header>
        <main className="main-content">
          {children}
          <footer className="page-footer">
            <span>云序 · 让协作井然有序</span>
            <span>
              安全连接 <Icon name="lock" size={12} />
            </span>
          </footer>
        </main>
      </div>
    </div>
  )
}
