import { Link, Navigate, Route, Routes } from 'react-router'
import { createApi } from './api/client'
import { Brand, ErrorNotice, Loading } from './components/ui'
import { useResource } from './hooks/async'
import { FullPage } from './layouts/FullPage'
import { SessionRoutes } from './router/SessionRoutes'

const api = createApi({})
const loadSite = (signal: AbortSignal) => api.site(signal)

/** 服务端根据 Host 确定入口；URL 只能选择当前域名允许的身份空间。 */
export default function App() {
  const site = useResource(loadSite)
  if (site.loading) return <FullPage><Loading label="正在连接服务…" /></FullPage>
  if (site.error) return <FullPage><Brand /><ErrorNotice message={site.error} retry={site.refresh} /></FullPage>
  if (!site.data || site.data.kind === 'unknown') return (
    <FullPage>
      <Brand />
      <h1>域名未绑定</h1>
      <p>当前域名尚未开通，请联系平台管理员。</p>
    </FullPage>
  )
  if (site.data.kind === 'platform') return (
    <Routes>
      <Route path="/platform/*" element={<SessionRoutes key="platform" platform />} />
      <Route path="/*" element={<Navigate to="/platform" replace />} />
    </Routes>
  )
  return (
    <Routes>
      <Route path="/platform/*" element={(
        <FullPage>
          <h1>无权访问</h1>
          <p>当前企业域名不提供平台管理入口。</p>
          <Link to="/">返回企业空间</Link>
        </FullPage>
      )} />
      <Route path="/*" element={<SessionRoutes key="tenant" platform={false} tenant={site.data.tenant} />} />
    </Routes>
  )
}
