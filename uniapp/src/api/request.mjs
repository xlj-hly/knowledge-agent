function apiError(code, message, status = 0) {
  return Object.assign(new Error(message), { code, status })
}

function isHostname(value) {
  return value.length <= 253 && value.split('.').every(label => /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(label))
}

export function resolveApiConfig(env = {}, browserHostname = '') {
  const configuredBase = String(env.VITE_API_BASE_URL || '').trim()
  if (!browserHostname && !configuredBase) {
    throw new Error('服务连接尚未配置，请联系企业管理员。')
  }
  const baseUrl = (configuredBase || '/api').replace(/\/+$/, '')
  const absolute = /^https?:\/\/([a-z0-9.-]+)(?::(\d{1,5}))?(?:\/[^\s?#]*)?$/i.exec(baseUrl)
  // 后端按实际请求域名识别企业；hostname 仅用于本地会话隔离，不允许客户端覆盖企业。
  const hostname = String(absolute?.[1] || browserHostname || '').trim().toLowerCase()
  const validAbsolute = absolute && isHostname(absolute[1].toLowerCase()) && (!absolute[2] || (Number(absolute[2]) > 0 && Number(absolute[2]) <= 65535))
  const validRelative = browserHostname && /^\/(?!\/)[^\s?#]*$/.test(baseUrl)
  if ((!validAbsolute && !validRelative) || !isHostname(hostname)) {
    throw new Error('服务连接配置无效，请联系企业管理员。')
  }
  return { baseUrl, hostname }
}

export function createTenantClient({ uni, baseUrl, hostname, onUnauthorized = () => {} }) {
  const storageKey = `saas.session:${baseUrl}:${hostname}`
  const pending = new Set()
  const invalidatedTokens = new Set()
  let revision = 0
  let session = null
  function readSession() {
    let stored
    try {
      stored = uni.getStorageSync(storageKey)
    } catch {
      throw apiError('STORAGE_ERROR', '无法读取本地登录信息，请检查存储权限后重试。')
    }
    return stored && typeof stored.token === 'string' && stored.token && typeof stored.expires_at === 'string'
      && !invalidatedTokens.has(stored.token) ? stored : null
  }
  session = readSession()

  function invalidateMemory() {
    if (session) invalidatedTokens.add(session.token)
    revision++
    session = null
  }

  function clearSession() {
    // Memory and request ownership are invalid before platform storage is touched.
    invalidateMemory()
    try {
      uni.removeStorageSync(storageKey)
    } catch {
      throw apiError('STORAGE_ERROR', '本地登录信息清理失败，请检查存储权限。')
    }
  }

  function invalidateSession(error) {
    invalidateMemory()
    // Notify the state owner even when storage or its notification callback fails.
    try {
      onUnauthorized(error)
    } catch (notificationError) {
      error.notificationError = notificationError
    } finally {
      try {
        uni.removeStorageSync(storageKey)
      } catch {
        error.storageError = apiError('STORAGE_ERROR', '本地登录信息清理失败，请检查存储权限。')
      }
    }
  }

  function getSession() {
    if (session && (!Number.isFinite(Date.parse(session.expires_at)) || Date.parse(session.expires_at) <= Date.now())) {
      invalidateSession(apiError('UNAUTHORIZED', '登录已失效，请重新登录。', 401))
    }
    return session
  }

  function saveSession(value) {
    if (!value || typeof value.token !== 'string' || !value.token || !Number.isFinite(Date.parse(value.expires_at))
      || Date.parse(value.expires_at) <= Date.now()
      || (value.tenant?.hostname && value.tenant.hostname.toLowerCase() !== hostname)) {
      throw new Error('登录响应无效，请重新登录。')
    }
    const next = { token: value.token, expires_at: value.expires_at }
    try {
      uni.setStorageSync(storageKey, next)
    } catch {
      throw apiError('STORAGE_ERROR', '无法保存本地登录信息，请检查存储权限后重试。')
    }
    revision++
    invalidatedTokens.delete(next.token)
    session = next
  }

  // 普通请求与头像上传共享归属、期限及错误处理；上传由平台设置 multipart 边界。
  function dispatch(path, { method = 'GET', data, authenticated = true, filePath } = {}) {
    const current = authenticated ? getSession() : null
    if (authenticated && !current) return Promise.reject(apiError('UNAUTHORIZED', '登录已失效，请重新登录。', 401))
    const requestRevision = revision
    return new Promise((resolve, reject) => {
      let settled = false
      let task
      let timer
      const finish = (error, value) => {
        if (settled) return
        settled = true
        clearTimeout(timer)
        pending.delete(cancel)
        if (error) reject(error)
        else resolve(value)
      }
      const abortTask = () => {
        try { task?.abort() } catch { /* 等待已经结束，平台 abort 失败不能恢复旧请求归属。 */ }
      }
      const cancel = () => {
        finish(apiError('CANCELLED', '请求已取消'))
        abortTask()
      }
      pending.add(cancel)
      timer = setTimeout(() => {
        finish(apiError('NETWORK_ERROR', '请求超时，请检查网络后重试。'))
        abortTask()
      }, 15000)
      const success = response => {
        if (settled) return
        try {
          if (authenticated && requestRevision !== revision) {
            finish(apiError('CANCELLED', '登录状态已变化'))
            return
          }
          const status = response.statusCode
          let body = response.data
          if (filePath && typeof body === 'string') {
            try { body = JSON.parse(body) } catch {
              finish(apiError('INVALID_RESPONSE', '服务响应格式无效，请稍后重试。'))
              return
            }
          }
          if (status < 200 || status >= 300) {
            const error = apiError(body?.error?.code || 'HTTP_ERROR', body?.error?.message || `请求失败（${status}），请稍后重试。`, status)
            // 微信 code 过期只需重新获取 code，不代表当前 SaaS Bearer 会话失效。
            if (authenticated && ((status === 401 && error.code !== 'wechat_invalid_code') || (status === 403 && error.code === 'disabled'))) invalidateSession(error)
            finish(error)
            return
          }
          if (!body || typeof body !== 'object' || !Object.prototype.hasOwnProperty.call(body, 'data')) {
            finish(apiError('INVALID_RESPONSE', '服务响应格式无效，请检查 API 地址或稍后重试。'))
            return
          }
          finish(null, body.data)
        } catch {
          finish(apiError('INVALID_RESPONSE', '服务响应处理失败，请重新连接。'))
        }
      }
      const fail = error => finish(apiError('NETWORK_ERROR', error?.errMsg?.includes('timeout') ? '请求超时，请检查网络后重试。' : '无法连接服务，请检查网络后重试。'))
      try {
        const options = {
          url: `${baseUrl}${path}`,
          timeout: 15000,
          header: { ...(current ? { Authorization: `Bearer ${current.token}` } : {}) },
          success,
          fail,
        }
        task = filePath
          ? uni.uploadFile({ ...options, filePath, name: 'file' })
          : uni.request({ ...options, method, data, header: { ...options.header, 'Content-Type': 'application/json' } })
      } catch {
        finish(apiError('NETWORK_ERROR', '请求未能发出，请稍后重试。'))
      }
    })
  }

  function avatarURL(path) {
    if (!path) return ''
    if (/^https?:\/\/[^\s]+$/i.test(path)) return path
    if (!/^\/api\/avatars\/[1-9]\d*\/[1-9]\d*\/[a-zA-Z0-9_-]+\.(?:png|jpg)$/.test(path)) return ''
    const origin = /^(https?:\/\/[^/]+)/i.exec(baseUrl)?.[1] || ''
    return `${origin}${path}`
  }

  return {
    request: (path, options) => dispatch(path, options),
    upload: (path, filePath) => dispatch(path, { filePath }),
    avatarURL, getSession, saveSession, clearSession,
    syncSession() {
      let stored
      try {
        stored = readSession()
      } catch (error) {
        revision++
        session = null
        throw error
      }
      if (stored?.token !== session?.token || stored?.expires_at !== session?.expires_at) {
        revision++
        session = stored
      }
      return getSession()
    },
    cancelAll() { [...pending].forEach(cancel => cancel()) },
  }
}
