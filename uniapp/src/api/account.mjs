const isID = value => typeof value === 'string' && /^[1-9]\d*$/.test(value)
const isTenant = tenant => tenant && isID(tenant.id) && typeof tenant.name === 'string'
  && typeof tenant.hostname === 'string' && tenant.status === 'active'
const isUser = user => user && isID(user.id) && isID(user.tenant_id) && typeof user.username === 'string'
  && typeof user.nickname === 'string' && typeof user.avatar_url === 'string'
  && ['admin', 'user'].includes(user.role) && user.status === 'active'

function validateAccount(account, tenant) {
  // 身份契约在持久化会话或展示授权内容前校验；BIGINT 标识始终保留字符串。
  if (!isUser(account?.user) || !isTenant(account?.tenant) || account.user.tenant_id !== account.tenant.id
    || (tenant && (account.tenant.id !== tenant.id || account.tenant.hostname !== tenant.hostname))) {
    throw new Error('账户响应无效，请重试。')
  }
  return account
}

export function createAccountApi(client) {
  return {
    async getTenant() {
      const tenant = await client.request('/tenant', { authenticated: false })
      if (!isTenant(tenant)) throw new Error('企业响应无效，请重新连接。')
      return tenant
    },
    async getAccount(tenant) {
      return validateAccount(await client.request('/auth/me'), tenant)
    },
    async login(data, tenant) {
      return validateAccount(await client.request('/auth/login', { method: 'POST', data, authenticated: false }), tenant)
    },
    async wechatLogin(code, tenant) {
      return validateAccount(await client.request('/auth/wechat/login', { method: 'POST', data: { code }, authenticated: false }), tenant)
    },
    async updateProfile(data, identity) {
      const user = await client.request('/account/profile', { method: 'PUT', data })
      if (!isUser(user) || user.id !== identity.user.id || user.tenant_id !== identity.tenant.id) {
        throw new Error('资料响应无效，请刷新账户后重试。')
      }
      return user
    },
    async uploadAvatar(filePath, identity) {
      if (typeof filePath !== 'string' || !filePath) throw new Error('请选择有效头像。')
      const result = await client.upload('/account/avatar', filePath)
      const prefix = `/api/avatars/${identity.tenant.id}/${identity.user.id}/`
      if (typeof result?.avatar_url !== 'string' || !result.avatar_url.startsWith(prefix)
        || !/^[a-zA-Z0-9_-]+\.(?:png|jpg)$/.test(result.avatar_url.slice(prefix.length))) {
        throw new Error('头像响应无效，请重新选择。')
      }
      return result.avatar_url
    },
    changePassword(data) {
      return client.request('/account/password', { method: 'POST', data })
    },
    changeWechatPassword(data) {
      return client.request('/account/wechat/password', { method: 'POST', data })
    },
    logout() {
      return client.request('/auth/logout', { method: 'POST' })
    },
  }
}
