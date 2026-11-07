export type PageName = 'overview' | 'chat' | 'tenants' | 'members' | 'settings' | 'account' | 'knowledge' | 'models' | 'embedders' | 'vector-stores' | 'ai-settings'

const segments: Record<PageName, string> = {
  overview: 'overview', chat: '', tenants: 'tenants', members: 'members', settings: 'settings', account: 'account',
  knowledge: 'knowledge', models: 'models', embedders: 'embedders', 'vector-stores': 'vector-stores', 'ai-settings': 'ai-settings',
}
const titles: Record<PageName, string> = {
  overview: '工作台', chat: '对话', tenants: '企业管理', members: '成员管理', settings: '企业设置', account: '个人账号',
  knowledge: '知识库', models: 'LLM 对话模型', embedders: '嵌入器 Embedder', 'vector-stores': '向量存储', 'ai-settings': '默认配置与提示词',
}

export function pagePath(platform: boolean, page: PageName) {
  const base = platform ? '/platform' : ''
  if (platform && page === 'overview') return base
  return segments[page] ? `${base}/${segments[page]}` : base || '/'
}

export function routeTitle(platform: boolean, pathname: string) {
  const current = pathname.replace(/\/+$/, '') || '/'
  if (!platform && /^\/knowledge\/[1-9]\d*$/.test(current)) return '知识库详情'
  if (!platform && /^\/chat\/[1-9]\d*$/.test(current)) return '对话'
  const available: PageName[] = platform ? ['overview', 'tenants', 'account'] : ['chat', 'overview', 'members', 'settings', 'account', 'knowledge', 'models', 'embedders', 'vector-stores', 'ai-settings']
  const page = available.find(name => pagePath(platform, name) === current)
  return page ? titles[page] : '页面不存在'
}

/** 登录只恢复所属身份空间的站内目标，状态不能将用户导航到外站或另一种身份。 */
export function loginDestination(platform: boolean, state: unknown) {
  const fallback = pagePath(platform, platform ? 'overview' : 'chat')
  if (!state || typeof state !== 'object' || !('from' in state) || typeof state.from !== 'string') return fallback
  const from = state.from
  const pathname = from.split(/[?#]/)[0].replace(/\/+$/, '') || '/'
  const allowed = platform ? ['/platform', '/platform/tenants', '/platform/account'] : ['/', '/overview', '/members', '/settings', '/account', '/knowledge', '/models', '/embedders', '/vector-stores', '/ai-settings']
  const detail = !platform && /^\/(knowledge|chat)\/[1-9]\d*$/.test(pathname)
  if (!from.startsWith('/') || from.startsWith('//') || (!allowed.includes(pathname) && !detail)) return fallback
  return from
}
