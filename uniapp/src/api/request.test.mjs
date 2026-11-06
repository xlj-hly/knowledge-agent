import { afterEach, test, expect, vi } from 'vitest'
import assert from 'node:assert/strict'
import { createTenantClient, resolveApiConfig } from './request.mjs'

afterEach(() => vi.useRealTimers())

function runtime() {
  const storage = new Map()
  const calls = []
  return {
    calls,
    getStorageSync: key => storage.get(key),
    setStorageSync: (key, value) => storage.set(key, value),
    removeStorageSync: key => storage.delete(key),
    request(options) {
      const call = { ...options, aborted: false }
      calls.push(call)
      return { abort() { call.aborted = true; options.fail({ errMsg: 'request:fail abort' }) } }
    },
  }
}

function login(token = 'token-a') {
  return { token, expires_at: '2099-01-01T00:00:00Z', user: { id: '7' }, tenant: { hostname: 'acme.test' } }
}

test('uses the H5 host and infers a mini program tenant from its own API domain', () => {
  assert.deepEqual(resolveApiConfig({}, 'acme.test'), { baseUrl: '/api', hostname: 'acme.test' })
  assert.deepEqual(resolveApiConfig({ VITE_API_BASE_URL: 'https://Acme.test:443/api/' }), { baseUrl: 'https://Acme.test:443/api', hostname: 'acme.test' })
  assert.deepEqual(resolveApiConfig({ VITE_API_BASE_URL: 'https://api.test/api' }, 'web.test'), { baseUrl: 'https://api.test/api', hostname: 'api.test' })
  assert.deepEqual(resolveApiConfig({ VITE_API_BASE_URL: 'https://api.test/api/', VITE_TENANT_HOSTNAME: 'Acme.test' }), {
    baseUrl: 'https://api.test/api', hostname: 'api.test',
  })
  assert.throws(() => resolveApiConfig({}), /服务连接尚未配置，请联系企业管理员。/)
  assert.throws(() => resolveApiConfig({ VITE_API_BASE_URL: 'invalid', VITE_TENANT_HOSTNAME: 'acme.test' }), /服务连接配置无效，请联系企业管理员。/)
})

test.each([
  'https://user:password@acme.test/api', 'https://acme.test:0/api', 'https://acme.test:65536/api',
  'https://acme.test/api?key=value', 'https://acme.test/api#hash', '//acme.test/api',
  'https://-bad.test/api', 'https://acme..test/api',
])('rejects malformed API addresses before sending credentials: %s', baseUrl => {
  expect(() => resolveApiConfig({ VITE_API_BASE_URL: baseUrl }, 'acme.test')).toThrow(/服务连接配置无效/)
})

test('keeps tenant sessions separate and rejects expired stored credentials', () => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date('2026-01-01T00:00:00Z'))
  const uni = runtime()
  const first = createTenantClient({ uni, baseUrl: '/api', hostname: 'acme.test' })
  first.saveSession(login())
  assert.equal(createTenantClient({ uni, baseUrl: '/api', hostname: 'other.test' }).getSession(), null)
  assert.equal(createTenantClient({ uni, baseUrl: '/api', hostname: 'acme.test' }).getSession().token, 'token-a')
  first.saveSession({ ...login(), expires_at: '2027-01-01T00:00:00Z' })
  vi.setSystemTime(new Date('2028-01-01T00:00:00Z'))
  assert.equal(first.getSession(), null)
})

test('lets the request domain identify the tenant and sends bearer credentials only for protected routes', async () => {
  const uni = runtime()
  const client = createTenantClient({ uni, baseUrl: '/api', hostname: 'acme.test' })
  client.saveSession(login())
  const tenant = client.request('/tenant', { authenticated: false })
  assert.equal(uni.calls[0].url, '/api/tenant')
  assert.equal(uni.calls[0].header['X-Tenant-Hostname'], undefined)
  assert.equal(uni.calls[0].header.Authorization, undefined)
  uni.calls[0].success({ statusCode: 200, data: { data: { name: '示例企业' } } })
  assert.equal((await tenant).name, '示例企业')
  const me = client.request('/auth/me')
  assert.equal(uni.calls[1].header.Authorization, 'Bearer token-a')
  uni.calls[1].success({ statusCode: 200, data: { data: { user: { id: '7' } } } })
  assert.equal((await me).user.id, '7')
})

test('an old unauthorized response cannot erase a newer login', async () => {
  const uni = runtime()
  let expirations = 0
  const client = createTenantClient({ uni, baseUrl: '/api', hostname: 'acme.test', onUnauthorized: () => expirations++ })
  client.saveSession(login())
  const old = client.request('/auth/me')
  client.saveSession(login('token-b'))
  uni.calls[0].success({ statusCode: 401, data: { error: { code: 'UNAUTHORIZED', message: '登录失效' } } })
  await assert.rejects(old, error => error.code === 'CANCELLED')
  assert.equal(client.getSession().token, 'token-b')
  assert.equal(expirations, 0)
  const current = client.request('/auth/me')
  uni.calls[1].success({ statusCode: 401, data: { error: { code: 'UNAUTHORIZED', message: '登录失效' } } })
  await assert.rejects(current, error => error.status === 401)
  assert.equal(client.getSession(), null)
  assert.equal(expirations, 1)
})

test('disabled authenticated sessions are cleared while ordinary forbidden operations retain login', async () => {
  const uni = runtime()
  let reason
  const client = createTenantClient({ uni, baseUrl: '/api', hostname: 'acme.test', onUnauthorized: error => { reason = error } })
  client.saveSession(login())
  const forbidden = client.request('/account/profile', { method: 'PUT' })
  uni.calls[0].success({ statusCode: 403, data: { error: { code: 'forbidden', message: '无权操作' } } })
  await assert.rejects(forbidden, error => error.code === 'forbidden')
  assert.equal(client.getSession().token, 'token-a')
  assert.equal(reason, undefined)
  const disabled = client.request('/auth/logout', { method: 'POST' })
  uni.calls[1].success({ statusCode: 403, data: { error: { code: 'disabled', message: '企业或账号已停用' } } })
  await assert.rejects(disabled, error => error.code === 'disabled')
  assert.equal(client.getSession(), null)
  assert.equal(reason.code, 'disabled')
})

test('cancellation aborts requests and ignores late results without clearing login', async () => {
  const uni = runtime()
  const client = createTenantClient({ uni, baseUrl: '/api', hostname: 'acme.test' })
  client.saveSession(login())
  const request = client.request('/account/profile', { method: 'PUT', data: { nickname: '新昵称', avatar_url: '' } })
  client.cancelAll()
  await assert.rejects(request, error => error.code === 'CANCELLED')
  assert.equal(uni.calls[0].aborted, true)
  uni.calls[0].success({ statusCode: 401, data: {} })
  assert.equal(client.getSession().token, 'token-a')
})

test('preserves business errors and rejects malformed successful responses', async () => {
  const uni = runtime()
  const client = createTenantClient({ uni, baseUrl: '/api', hostname: 'acme.test' })
  const failed = client.request('/auth/register', { authenticated: false })
  uni.calls[0].success({ statusCode: 403, data: { error: { code: 'REGISTRATION_DISABLED', message: '企业已关闭注册' } } })
  await assert.rejects(failed, error => error.message === '企业已关闭注册' && error.code === 'REGISTRATION_DISABLED')
  const malformed = client.request('/tenant', { authenticated: false })
  uni.calls[1].success({ statusCode: 200, data: '<html>not API</html>' })
  await assert.rejects(malformed, /响应格式/)
})

test('accepts API responses on mini program engines without Object.hasOwn', async () => {
  const uni = runtime()
  const client = createTenantClient({ uni, baseUrl: '/api', hostname: 'acme.test' })
  const request = client.request('/tenant', { authenticated: false })
  const original = Object.hasOwn
  try {
    Object.hasOwn = undefined
    uni.calls[0].success({ statusCode: 200, data: { data: { name: '企业' } } })
    assert.equal((await request).name, '企业')
  } finally {
    Object.hasOwn = original
  }
})

test('storage removal failure cannot strand a response or retain an authenticated view', async () => {
  const uni = runtime()
  let reason
  const client = createTenantClient({ uni, baseUrl: '/api', hostname: 'acme.test', onUnauthorized: error => { reason = error } })
  client.saveSession(login())
  uni.removeStorageSync = () => { throw new Error('storage unavailable') }
  const request = client.request('/auth/me')
  expect(() => uni.calls[0].success({ statusCode: 401, data: { error: { code: 'UNAUTHORIZED', message: '登录失效' } } })).not.toThrow()
  await expect(request).rejects.toMatchObject({ status: 401 })
  expect(client.getSession()).toBeNull()
  expect(reason.code).toBe('UNAUTHORIZED')
})

test('failed storage removal cannot restore an explicitly cleared token on page resume', () => {
  const uni = runtime()
  const client = createTenantClient({ uni, baseUrl: '/api', hostname: 'acme.test' })
  client.saveSession(login())
  uni.removeStorageSync = () => { throw new Error('disk failure') }
  expect(() => client.clearSession()).toThrow(/清理失败/)
  expect(client.getSession()).toBeNull()
  expect(client.syncSession()).toBeNull()
})

test('storage read and write failures expose actionable errors without creating a memory session', () => {
  const uni = runtime()
  uni.getStorageSync = () => { throw new Error('disk failure') }
  expect(() => createTenantClient({ uni, baseUrl: '/api', hostname: 'acme.test' })).toThrow(/读取本地登录信息/)
  uni.getStorageSync = () => undefined
  const client = createTenantClient({ uni, baseUrl: '/api', hostname: 'acme.test' })
  uni.setStorageSync = () => { throw new Error('disk full') }
  expect(() => client.saveSession(login())).toThrow(/保存本地登录信息/)
  expect(client.getSession()).toBeNull()
})

test('notification exceptions in a platform response still settle the request and invalidate memory', async () => {
  const uni = runtime()
  const client = createTenantClient({ uni, baseUrl: '/api', hostname: 'acme.test', onUnauthorized: () => { throw new Error('observer failed') } })
  client.saveSession(login())
  const request = client.request('/auth/me')
  expect(() => uni.calls[0].success({ statusCode: 401, data: { error: { code: 'UNAUTHORIZED', message: '登录失效' } } })).not.toThrow()
  await expect(request).rejects.toMatchObject({ status: 401 })
  expect(client.getSession()).toBeNull()
})


test('avatar uploads share bearer ownership, decode the upload envelope and resolve stored image paths against API origin', async () => {
  const uni = runtime()
  const uploads = []
  uni.uploadFile = options => { uploads.push(options); return { abort() {} } }
  const client = createTenantClient({ uni, baseUrl: 'https://acme.test/api', hostname: 'acme.test' })
  client.saveSession(login())
  const pending = client.upload('/account/avatar', '/tmp/avatar.png')
  expect(uploads[0]).toMatchObject({ url: 'https://acme.test/api/account/avatar', name: 'file', filePath: '/tmp/avatar.png', header: { Authorization: 'Bearer token-a' } })
  expect(uploads[0].header['Content-Type']).toBeUndefined()
  expect(uploads[0].header['X-Tenant-Hostname']).toBeUndefined()
  uploads[0].success({ statusCode: 200, data: JSON.stringify({ data: { avatar_url: '/api/avatars/2/7/a.png' } }) })
  await expect(pending).resolves.toEqual({ avatar_url: '/api/avatars/2/7/a.png' })
  expect(client.avatarURL('/api/avatars/2/7/a.png')).toBe('https://acme.test/api/avatars/2/7/a.png')
})

test('an old upload cannot invalidate a newer identity and explicit cancellation aborts the upload', async () => {
  const uni = runtime()
  const uploads = []
  uni.uploadFile = options => { const upload = { ...options, aborted: false }; uploads.push(upload); return { abort() { upload.aborted = true } } }
  const client = createTenantClient({ uni, baseUrl: '/api', hostname: 'acme.test' })
  client.saveSession(login())
  const first = client.upload('/account/avatar', '/tmp/avatar.png')
  client.saveSession(login('token-b'))
  uploads[0].success({ statusCode: 401, data: JSON.stringify({ error: { code: 'unauthorized', message: '会话失效' } }) })
  await expect(first).rejects.toMatchObject({ code: 'CANCELLED' })
  expect(client.getSession().token).toBe('token-b')
  const second = client.upload('/account/avatar', '/tmp/avatar.png')
  client.cancelAll()
  await expect(second).rejects.toMatchObject({ code: 'CANCELLED' })
  expect(uploads[1].aborted).toBe(true)
})

test('uploads time out without a platform callback and malformed upload JSON never becomes a saved avatar', async () => {
  vi.useFakeTimers()
  const uni = runtime()
  const uploads = []
  uni.uploadFile = options => { uploads.push(options); return { abort() {} } }
  const client = createTenantClient({ uni, baseUrl: '/api', hostname: 'acme.test' })
  client.saveSession(login())
  const first = client.upload('/account/avatar', '/tmp/avatar.png')
  const firstCheck = expect(first).rejects.toMatchObject({ code: 'NETWORK_ERROR' })
  await vi.advanceTimersByTimeAsync(15000)
  await firstCheck
  const second = client.upload('/account/avatar', '/tmp/avatar.png')
  uploads[1].success({ statusCode: 200, data: '<html>not JSON</html>' })
  await expect(second).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
})


test('an expired WeChat verification code does not invalidate the still valid SaaS session', async () => {
  const uni = runtime()
  const client = createTenantClient({ uni, baseUrl: '/api', hostname: 'acme.test' })
  client.saveSession(login())
  const request = client.request('/account/wechat/password', { method: 'POST', data: { code: 'expired', new_password: 'newpassword1' } })
  uni.calls[0].success({ statusCode: 401, data: { error: { code: 'wechat_invalid_code', message: '微信凭证已失效，请重试。' } } })
  await expect(request).rejects.toMatchObject({ code: 'wechat_invalid_code' })
  expect(client.getSession().token).toBe('token-a')
})
