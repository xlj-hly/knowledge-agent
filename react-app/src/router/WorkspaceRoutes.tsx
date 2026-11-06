import { useState } from 'react'
import { Route, Routes, useLocation } from 'react-router'
import type { Api } from '../api/client'
import type { Identity } from '../stores/session/types'
import type { NavItemProps } from '../layouts/NavItem'
import { useAction } from '../hooks/async'
import { ErrorNotice } from '../components/ui'
import { WorkspaceLayout } from '../layouts/WorkspaceLayout'
import { OverviewPage } from '../pages/overview/OverviewPage'
import { TenantsPage } from '../pages/platform/tenants/TenantsPage'
import { MembersPage } from '../pages/workspace/members/MembersPage'
import { AccountPage } from '../pages/account/AccountPage'
import { SettingsPage } from '../pages/workspace/settings/SettingsPage'
import { RouteErrorPage } from '../pages/error/RouteErrorPage'
import { pagePath, routeTitle } from './navigation'

/** 会话决定工作空间生命周期，URL 决定当前页面；页面草稿随路由卸载失效。 */
export function WorkspaceRoutes({ api, platform, identity, onSessionEnd, sessionError = '' }: {
  api: Api
  platform: boolean
  identity: Identity
  onSessionEnd: (notice: string) => void
  sessionError?: string
}) {
  const [updatedIdentity, setIdentity] = useState<Identity | null>(null)
  const current = updatedIdentity ?? identity
  const admin = 'admin' in current ? current.admin : null
  const user = 'user' in current ? current.user : null
  const tenant = 'tenant' in current ? current.tenant : null
  const location = useLocation()
  const logout = useAction()
  const signOut = () => {
    void logout.run(
      (signal) => platform ? api.platformLogout(signal) : api.logout(signal),
      () => onSessionEnd('已安全退出当前会话。'),
    )
  }
  const home = pagePath(platform, 'overview')
  const account = pagePath(platform, 'account')
  const navigation: NavItemProps[] = [{ icon: 'grid', label: '工作台', to: home, end: true }]
  if (admin) navigation.push({ icon: 'building', label: '企业管理', to: pagePath(platform, 'tenants') })
  if (user?.role === 'admin') {
    navigation.push(
      { icon: 'users', label: '成员管理', to: pagePath(platform, 'members') },
      { icon: 'settings', label: '企业设置', to: pagePath(platform, 'settings') },
    )
  }
  return (
    <WorkspaceLayout
      identity={current}
      platform={platform}
      homePath={home}
      accountPath={account}
      navigation={navigation}
      title={routeTitle(platform, location.pathname)}
      onSignOut={signOut}
      signingOut={logout.busy}
    >
      <ErrorNotice message={sessionError} />
      <ErrorNotice message={logout.error} />
      <Routes>
        <Route
          index
          element={<OverviewPage admin={admin} user={user} tenant={tenant} platform={platform} />}
        />
        {platform ? (
          <Route path="tenants" element={<TenantsPage api={api} />} />
        ) : (
          <>
            <Route path="members" element={user?.role === 'admin' ? (
              <MembersPage
                api={api}
                currentUser={user}
                onSelfChanged={() => onSessionEnd('你的账号权限或密码已更新，请重新登录。')}
              />
            ) : <RouteErrorPage forbidden home={home} />} />
            <Route path="settings" element={user?.role === 'admin' ? (
              <SettingsPage api={api} onChanged={setIdentity} />
            ) : <RouteErrorPage forbidden home={home} />} />
          </>
        )}
        <Route
          path="account"
          element={<AccountPage api={api} platform={platform} onChanged={setIdentity} onSessionEnd={onSessionEnd} />}
        />
        <Route path="*" element={<RouteErrorPage home={home} />} />
      </Routes>
    </WorkspaceLayout>
  )
}
