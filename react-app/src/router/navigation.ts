export type PageName = 'overview' | 'tenants' | 'members' | 'settings' | 'account'

const segments: Record<PageName, string> = {
  overview: '', tenants: 'tenants', members: 'members', settings: 'settings', account: 'account',
}
const titles: Record<PageName, string> = {
  overview: '工作台', tenants: '企业管理', members: '成员管理', settings: '企业设置', account: '个人账号',
}

export function pagePath(platform: boolean, page: PageName) {
  const base = platform ? '/platform' : ''
  return segments[page] ? `${base}/${segments[page]}` : base || '/'
}

export function routeTitle(platform: boolean, pathname: string) {
  const current = pathname.replace(/\/+$/, '') || '/'
  const available: PageName[] = platform ? ['overview', 'tenants', 'account'] : ['overview', 'members', 'settings', 'account']
  const page = available.find(name => pagePath(platform, name) === current)
  return page ? titles[page] : '页面不存在'
}

/** 登录只恢复所属身份空间的站内目标，状态不能将用户导航到外站或另一种身份。 */
export function loginDestination(platform: boolean, state: unknown) {
  const fallback = pagePath(platform, 'overview')
  if (!state || typeof state !== 'object' || !('from' in state) || typeof state.from !== 'string') return fallback
  const from = state.from
  const pathname = from.split(/[?#]/)[0].replace(/\/+$/, '') || '/'
  const allowed = platform ? ['/platform', '/platform/tenants', '/platform/account'] : ['/', '/members', '/settings', '/account']
  if (!from.startsWith('/') || from.startsWith('//') || !allowed.includes(pathname)) return fallback
  return from
}
