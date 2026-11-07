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
import { KnowledgePage } from '../pages/knowledge/KnowledgePage'
import { KnowledgeDetailPage } from '../pages/knowledge/KnowledgeDetailPage'
import { ModelsPage } from '../pages/models/ModelsPage'
import { VectorStoresPage } from '../pages/vector-stores/VectorStoresPage'
import { ChatPage } from '../pages/chat/ChatPage'
import { AISettingsPage } from '../pages/ai-settings/AISettingsPage'
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
  const home = pagePath(platform, platform ? 'overview' : 'chat')
  const account = pagePath(platform, 'account')
  const navigation: NavItemProps[] = [{ icon: platform ? 'grid' : 'chat', label: platform ? '工作台' : '对话', to: home, end: true }]
  if (admin) navigation.push({ icon: 'building', label: '企业管理', to: pagePath(platform, 'tenants') })
  if (user) navigation.push({ icon: 'book', label: '知识库', to: pagePath(false, 'knowledge') })
  if (user?.role === 'admin') {
    navigation.push(
      { icon: 'spark', label: 'LLM 对话模型', to: pagePath(false, 'models'), group: 'ai' },
      { icon: 'spark', label: '嵌入器 Embedder', to: pagePath(false, 'embedders'), group: 'ai' },
      { icon: 'database', label: '向量存储', to: pagePath(false, 'vector-stores'), group: 'ai' },
      { icon: 'settings', label: '默认配置与提示词', to: pagePath(false, 'ai-settings'), group: 'ai' },
      { icon: 'users', label: '成员管理', to: pagePath(platform, 'members'), group: 'management' },
      { icon: 'settings', label: '企业设置', to: pagePath(platform, 'settings'), group: 'management' },
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
          element={platform ? <OverviewPage admin={admin} user={user} tenant={tenant} platform={platform} /> : <ChatPage api={api} />}
        />
        {platform ? (
          <Route path="tenants" element={<TenantsPage api={api} />} />
        ) : (
          <>
            <Route path="chat/:id" element={<ChatPage api={api} />} />
            <Route path="overview" element={<OverviewPage admin={admin} user={user} tenant={tenant} platform={false} />} />
            <Route path="knowledge" element={<KnowledgePage api={api} />} />
            <Route path="knowledge/:id" element={<KnowledgeDetailPage api={api} />} />
            <Route path="models" element={user?.role === 'admin' ? <ModelsPage key="chat" api={api} kind="chat" /> : <RouteErrorPage forbidden home={home} />} />
            <Route path="embedders" element={user?.role === 'admin' ? <ModelsPage key="embedding" api={api} kind="embedding" /> : <RouteErrorPage forbidden home={home} />} />
            <Route path="ai-settings" element={user?.role === 'admin' ? <AISettingsPage api={api} /> : <RouteErrorPage forbidden home={home} />} />
            <Route path="vector-stores" element={user?.role === 'admin' ? <VectorStoresPage api={api} /> : <RouteErrorPage forbidden home={home} />} />
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
