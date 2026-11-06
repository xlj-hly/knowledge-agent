import { useCallback } from 'react'
import { Navigate, Route, Routes, useLocation } from 'react-router'
import { LoginPage } from '../pages/login/LoginPage'
import { Brand, ErrorNotice, Loading } from '../components/ui'
import { useResource } from '../hooks/async'
import { FullPage } from '../layouts/FullPage'
import { useSession } from '../stores/session/useSession'
import { WorkspaceRoutes } from './WorkspaceRoutes'
import { loginDestination, pagePath } from './navigation'
import type { Tenant } from '../api/tenants/types'

/** 本身份空间的路由先验证服务端身份，再开放登录或工作空间入口。 */
export function SessionRoutes({ platform, tenant }: { platform: boolean; tenant?: Tenant }) {
  const location = useLocation()
  const hostname = window.location.hostname
  const storageKey = platform ? `yunxu.platform.session.${hostname}` : `yunxu.tenant.session.${hostname}`
  const session = useSession({ storageKey })
  const { api, token } = session
  const load = useCallback(
    async (signal: AbortSignal) => {
      if (!token) return null
      return platform ? api.platformMe(signal) : api.me(signal)
    },
    [api, token, platform],
  )
  const identity = useResource(load)
  if (token && identity.loading)
    return (
      <FullPage>
        <ErrorNotice message={session.storageNotice} />
        <Loading label="正在验证登录状态…" />
      </FullPage>
    )
  if (token && identity.error)
    return (
      <FullPage>
        <Brand platform={platform} />
        <ErrorNotice message={session.storageNotice} />
        <ErrorNotice message={identity.error} retry={identity.refresh} />
        <button className="button secondary" onClick={() => session.changeToken('')}>
          返回登录
        </button>
      </FullPage>
    )
  const authenticated = Boolean(token && identity.data)
  const loginPath = platform ? '/platform/login' : '/login'
  return (
    <Routes>
      <Route
        path="login"
        element={authenticated ? (
          <Navigate to={loginDestination(platform, location.state)} replace />
        ) : (
          <LoginPage
            key={session.version}
            api={api}
            platform={platform}
            tenant={tenant}
            homePath={pagePath(platform, 'overview')}
            notice={session.notice}
            sessionError={session.storageNotice}
            onLogin={session.changeToken}
          />
        )}
      />
      <Route
        path="*"
        element={authenticated && identity.data ? (
          <WorkspaceRoutes
            key={session.version}
            api={api}
            platform={platform}
            identity={identity.data}
            sessionError={session.storageNotice}
            onSessionEnd={(notice) => {
              session.changeToken('')
              session.setNotice(notice)
            }}
          />
        ) : (
          <Navigate
            to={loginPath}
            state={{ from: location.pathname + location.search + location.hash }}
            replace
          />
        )}
      />
    </Routes>
  )
}
