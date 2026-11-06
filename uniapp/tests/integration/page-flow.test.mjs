import { afterEach, expect, test, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { reactive } from 'vue'
import { createTenantClient } from '../../src/api/request.mjs'
import { createAccountController, createAccountState } from '../../src/pages/index/state.mjs'
import IndexPage from '../../src/pages/index/index.vue'

const tenant = { id: '2', hostname: 'acme.test', name: '示例企业', status: 'active' }
const user = { id: '7', username: 'alice', nickname: '原昵称', avatar_url: '', role: 'user', tenant_id: '2', status: 'active' }
const wrappers = []
afterEach(() => {
  wrappers.splice(0).forEach(wrapper => wrapper.unmount())
  vi.useRealTimers()
  vi.unstubAllGlobals()
  vi.unstubAllEnvs()
})

function platform() {
  const storage = new Map()
  const requests = []
  const logins = []
  const uploads = []
  const images = []
  const uni = {
    getStorageSync: key => storage.get(key),
    setStorageSync: (key, value) => storage.set(key, value),
    removeStorageSync: key => storage.delete(key),
    request(request) {
      requests.push(request)
      return { abort: () => request.fail({ errMsg: 'request:fail abort' }) }
    },
    login(options) { logins.push(options) },
    uploadFile(options) { uploads.push(options); return { abort() {} } },
    chooseImage(options) { images.push(options) },
  }
  const respond = (index, data, statusCode = 200) => requests[index].success({ statusCode, data: statusCode === 200 ? { data } : { error: data } })
  return { uni, requests, storage, respond, logins, uploads, images }
}

function setup() {
  const runtime = platform()
  vi.stubGlobal('uni', runtime.uni)
  const state = reactive(createAccountState())
  let controller
  const client = createTenantClient({ uni: runtime.uni, baseUrl: '/api', hostname: tenant.hostname, onUnauthorized: error => controller.sessionExpired(error) })
  controller = createAccountController({ state, client })
  state.tenant = { ...tenant }
  state.ready = true
  state.loading = false
  // 交互套件直接装配受控初态；完整页面与真实控制器照常参与 Vue 渲染。
  const wrapper = mount(IndexPage, { data: () => ({ account: state, controller }) })
  wrappers.push(wrapper)
  return { state, controller, client, wrapper, ...runtime }
}

async function setupPage({ wechat = false, session = true, tenantValue = tenant } = {}) {
  const runtime = platform()
  const lifecycle = {}
  const storageKey = `saas.session:${wechat ? 'https://acme.test/api' : '/api'}:acme.test`
  if (session) runtime.storage.set(storageKey, { token: 'token-a', expires_at: '2099-01-01T00:00:00Z' })
  vi.stubGlobal('uni', runtime.uni)
  vi.stubEnv('UNI_PLATFORM', wechat ? 'mp-weixin' : 'h5')
  vi.stubGlobal('location', { hostname: 'acme.test' })
  vi.stubEnv('VITE_API_BASE_URL', wechat ? 'https://acme.test/api' : '/api')
  const wrapper = mount(IndexPage, {
    global: {
      mixins: [{
        mounted() {
          // 仅替换平台钩子的分发；页面实例、Vue 生命周期、渲染与业务控制器均真实执行。
          if (!this.$options.onLoad) return
          lifecycle.load = () => this.$options.onLoad.call(this)
          lifecycle.show = () => this.$options.onShow.call(this)
          lifecycle.unload = () => this.$options.onUnload.call(this)
        },
      }],
    },
  })
  wrappers.push(wrapper)
  lifecycle.load()
  runtime.respond(0, tenantValue)
  await flushPromises()
  if (session) {
    runtime.respond(1, { user, tenant: tenantValue })
    await flushPromises()
  }
  return { ...runtime, wrapper, lifecycle, storageKey, state: wrapper.vm.account }
}

async function click(wrapper, label) {
  const button = wrapper.findAll('button').find(button => button.text() === label || button.text().startsWith(label))
  expect(button, label).toBeDefined()
  if (button.attributes('form-type') === 'submit') {
    // 平台 form-type 按钮会触发表单 submit；实际表单组件和保存链路继续参与验证。
    await wrapper.get('form').trigger('submit', { detail: { value: { nickname: wrapper.get('#nickname').element.value } } })
  } else await button.trigger('click')
  await flushPromises()
}

async function input(wrapper, selector, value) {
  await wrapper.get(selector).trigger('input', { detail: { value } })
}

function authenticated(context) {
  context.client.saveSession({ token: 'token-a', expires_at: '2099-01-01T00:00:00Z', tenant })
  context.state.user = { ...user }
  context.state.screen = 'home'
}

test('password login has no registration entry and a cancelled response cannot restore a session', async () => {
  const context = setup()
  expect(context.wrapper.text()).not.toContain('注册')
  await input(context.wrapper, '#username', 'Alice')
  await input(context.wrapper, '#password', 'strongpassword1')
  await click(context.wrapper, '登录')
  expect(context.requests[0].url).toBe('/api/auth/login')
  expect(context.requests[0].data).toEqual({ username: 'alice', password: 'strongpassword1' })
  context.controller.cancelPending()
  context.respond(0, { user, tenant, token: 'token-a', expires_at: '2099-01-01T00:00:00Z' })
  await flushPromises()
  expect(context.requests).toHaveLength(1)
  expect(context.state.user).toBeNull()
  expect(context.client.getSession()).toBeNull()
})

test('profile save uses PUT and password change clears all authenticated views', async () => {
  const context = setup()
  authenticated(context)
  await flushPromises()
  await click(context.wrapper, '编辑个人资料')
  await input(context.wrapper, '#nickname', '新昵称')
  await click(context.wrapper, '选择头像')
  context.images[0].success({ tempFilePaths: ['/tmp/avatar.png'] })
  await flushPromises()
  context.uploads[0].success({ statusCode: 200, data: JSON.stringify({ data: { avatar_url: '/api/avatars/2/7/avatar.png' } }) })
  await flushPromises()
  await click(context.wrapper, '保存资料')
  expect(context.requests[0].method).toBe('PUT')
  expect(context.requests[0].data).toEqual({ nickname: '新昵称', avatar_url: '/api/avatars/2/7/avatar.png' })
  context.respond(0, { ...user, nickname: '新昵称', avatar_url: '/api/avatars/2/7/avatar.png' })
  await flushPromises()
  expect(context.wrapper.text()).toContain('新昵称')
  expect(context.state.screen).toBe('home')
  await click(context.wrapper, '修改登录密码')
  await input(context.wrapper, '#current-password', 'strongpassword1')
  await input(context.wrapper, '#new-password', 'changedpassword1')
  await input(context.wrapper, '#confirm-password', 'changedpassword1')
  await click(context.wrapper, '确认修改密码')
  expect(context.requests[1].data).toEqual({ current_password: 'strongpassword1', new_password: 'changedpassword1' })
  context.respond(1, { ok: true })
  await flushPromises()
  expect(context.state.user).toBeNull()
  expect(context.state.screen).toBe('login')
  expect(context.client.getSession()).toBeNull()
  expect(context.state.passwords.new_password).toBe('')
  expect(context.wrapper.text()).toContain('所有设备')
  expect(context.wrapper.find('#username').exists()).toBe(true)
})

test('failed logout retains a retryable session and unauthorized response returns to login', async () => {
  const context = setup()
  authenticated(context)
  await flushPromises()
  await click(context.wrapper, '退出登录')
  context.requests[0].fail({ errMsg: 'request:fail network' })
  await flushPromises()
  expect(context.state.user.id).toBe('7')
  expect(context.client.getSession().token).toBe('token-a')
  expect(context.wrapper.get('[role="alert"]').text()).toContain('网络')
  await click(context.wrapper, '退出登录')
  context.respond(1, { code: 'UNAUTHORIZED', message: '会话失效' }, 401)
  await flushPromises()
  expect(context.state.user).toBeNull()
  expect(context.state.screen).toBe('login')
  expect(context.state.busy).toBe(false)
  expect(context.client.getSession()).toBeNull()
  expect(context.wrapper.get('[role="status"]').text()).toContain('登录已失效')
})

test('disabled enterprise hides authenticated actions and shows server reason', async () => {
  const context = setup()
  authenticated(context)
  await flushPromises()
  await click(context.wrapper, '退出登录')
  context.respond(0, { code: 'disabled', message: '企业或账号已停用' }, 403)
  await flushPromises()
  expect(context.state.user).toBeNull()
  expect(context.state.ready).toBe(false)
  expect(context.client.getSession()).toBeNull()
  expect(context.wrapper.get('[role="alert"]').text()).toContain('停用')
  expect(context.wrapper.text()).not.toContain('编辑个人资料')
})

test('restoring a different stored identity clears old forms and ignores old profile responses', async () => {
  const context = setup()
  authenticated(context)
  await context.wrapper.vm.$nextTick()
  await click(context.wrapper, '编辑个人资料')
  await input(context.wrapper, '#nickname', '上一账号草稿')
  await click(context.wrapper, '保存资料')
  context.state.passwords.current_password = 'oldsecret'
  const key = [...context.storage.keys()][0]
  context.storage.set(key, { token: 'token-b', expires_at: '2099-01-01T00:00:00Z' })
  const refresh = context.controller.refresh()
  expect(context.state.user).toBeNull()
  expect(context.state.profile.nickname).toBe('')
  expect(context.state.passwords.current_password).toBe('')
  context.respond(0, { ...user, nickname: '旧响应' })
  context.respond(1, tenant)
  await flushPromises()
  context.respond(2, { user: { ...user, id: '8', username: 'bob', nickname: '新账号' }, tenant })
  await refresh
  await flushPromises()
  expect(context.state.user.id).toBe('8')
  expect(context.wrapper.text()).toContain('新账号')
  expect(context.wrapper.text()).not.toContain('旧响应')
  await click(context.wrapper, '编辑个人资料')
  expect(context.wrapper.get('#nickname').element.value).toBe('新账号')
})

test('expired session clears every draft even when storage removal fails', async () => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date('2026-01-01T00:00:00Z'))
  const context = setup()
  authenticated(context)
  context.state.profile.nickname = '私密草稿'
  context.state.credentials.username = 'previous.user'
  context.state.passwords.new_password = 'secretpassword'
  context.client.saveSession({ token: 'expired', expires_at: '2027-01-01T00:00:00Z', tenant })
  vi.setSystemTime(new Date('2028-01-01T00:00:00Z'))
  context.uni.removeStorageSync = () => { throw new Error('disk failure') }
  const refresh = context.controller.refresh()
  expect(context.state.user).toBeNull()
  expect(context.state.profile.nickname).toBe('')
  expect(context.state.credentials.username).toBe('')
  expect(context.state.passwords.new_password).toBe('')
  context.respond(0, tenant)
  await refresh
  expect(context.state.busy).toBe(false)
  expect(context.client.getSession()).toBeNull()
})

test('login storage write failure finishes submission and keeps authorized actions hidden', async () => {
  const context = setup()
  context.uni.setStorageSync = () => { throw new Error('disk full') }
  await input(context.wrapper, '#username', 'Alice')
  await input(context.wrapper, '#password', 'strongpassword1')
  await click(context.wrapper, '登录')
  expect(context.requests[0].data).toEqual({ username: 'alice', password: 'strongpassword1' })
  context.respond(0, { token: 'token-a', expires_at: '2099-01-01T00:00:00Z', user, tenant })
  await flushPromises()
  expect(context.state.busy).toBe(false)
  expect(context.state.user).toBeNull()
  expect(context.client.getSession()).toBeNull()
  expect(context.wrapper.get('[role="alert"]').text()).toContain('保存本地登录信息')
  expect(context.wrapper.text()).not.toContain('编辑个人资料')
})

test('page resume storage read failure clears authenticated drafts and exposes a retry', async () => {
  const context = setup()
  authenticated(context)
  context.state.profile.nickname = '旧账号草稿'
  context.uni.getStorageSync = () => { throw new Error('disk unavailable') }
  await context.controller.refresh()
  await flushPromises()
  expect(context.state.user).toBeNull()
  expect(context.state.profile.nickname).toBe('')
  expect(context.client.getSession()).toBeNull()
  expect(context.wrapper.get('[role="alert"]').text()).toContain('读取本地登录信息')
  expect(context.wrapper.text()).toContain('重新连接')
  expect(context.requests).toHaveLength(0)
})

test('successful logout hides the account and finishes even when storage removal fails', async () => {
  const context = setup()
  authenticated(context)
  context.state.profile.nickname = '旧草稿'
  context.uni.removeStorageSync = () => { throw new Error('disk unavailable') }
  await flushPromises()
  await click(context.wrapper, '退出登录')
  context.respond(0, { ok: true })
  await flushPromises()
  expect(context.state.user).toBeNull()
  expect(context.state.busy).toBe(false)
  expect(context.state.profile.nickname).toBe('')
  expect(context.client.getSession()).toBeNull()
  expect(context.wrapper.get('[role="alert"]').text()).toContain('清理失败')
  expect(context.wrapper.find('#username').exists()).toBe(true)
})

test.each([
  { user: { ...user, id: 7 }, tenant },
  { user, tenant: { ...tenant, id: 2 } },
  { user: { ...user, tenant_id: '99' }, tenant },
  { user, tenant: { ...tenant, hostname: null } },
])('invalid login identity is rejected before persistence or authorized rendering: %j', async account => {
  const context = setup()
  await input(context.wrapper, '#username', 'Alice')
  await input(context.wrapper, '#password', 'strongpassword1')
  await click(context.wrapper, '登录')
  context.respond(0, { ...account, token: 'token-a', expires_at: '2099-01-01T00:00:00Z' })
  await flushPromises()
  expect(context.state.user).toBeNull()
  expect(context.client.getSession()).toBeNull()
  expect(context.storage.size).toBe(0)
  expect(context.wrapper.get('[role="alert"]').text()).toContain('响应无效')
})

test('successful login preserves a BIGINT user ID as a string', async () => {
  const context = setup()
  await input(context.wrapper, '#username', 'Alice')
  await input(context.wrapper, '#password', 'strongpassword1')
  await click(context.wrapper, '登录')
  context.respond(0, { user: { ...user, id: '9007199254740993' }, tenant, token: 'token-a', expires_at: '2099-01-01T00:00:00Z' })
  await flushPromises()
  expect(context.state.user.id).toBe('9007199254740993')
  expect(context.client.getSession().token).toBe('token-a')
  expect(context.wrapper.text()).toContain('编辑个人资料')
  expect(context.state.credentials.password).toBe('')
})

test('an already expired login response never exposes an authenticated view', async () => {
  const context = setup()
  await input(context.wrapper, '#username', 'Alice')
  await input(context.wrapper, '#password', 'strongpassword1')
  await click(context.wrapper, '登录')
  context.respond(0, { user, tenant, token: 'expired-token', expires_at: '2000-01-01T00:00:00Z' })
  await flushPromises()
  expect(context.state.user).toBeNull()
  expect(context.state.screen).toBe('login')
  expect(context.state.busy).toBe(false)
  expect(context.client.getSession()).toBeNull()
  expect(context.wrapper.text()).not.toContain('编辑个人资料')
  expect(context.wrapper.find('#username').exists()).toBe(true)
})

test('expiration during storage persistence cannot restore the authorized view after invalidation', async () => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date('2026-01-01T00:00:00Z'))
  const context = setup()
  const persist = context.uni.setStorageSync
  context.uni.setStorageSync = (key, value) => {
    persist(key, value)
    vi.setSystemTime(new Date('2026-01-01T00:00:02Z'))
  }
  await input(context.wrapper, '#username', 'Alice')
  await input(context.wrapper, '#password', 'strongpassword1')
  await click(context.wrapper, '登录')
  context.respond(0, { user, tenant, token: 'expired-token', expires_at: '2026-01-01T00:00:01Z' })
  await flushPromises()
  expect(context.state.user).toBeNull()
  expect(context.state.screen).toBe('login')
  expect(context.state.busy).toBe(false)
  expect(context.client.getSession()).toBeNull()
  expect(context.state.profile.nickname).toBe('')
  expect(context.wrapper.text()).not.toContain('编辑个人资料')
})

test('page onShow replaces the owner and discards a pending old profile write', async () => {
  const context = await setupPage()
  await click(context.wrapper, '编辑个人资料')
  await input(context.wrapper, '#nickname', '旧账号写入')
  await click(context.wrapper, '保存资料')
  expect(context.state.busy).toBe(true)
  context.storage.set('saas.session:/api:acme.test', { token: 'token-b', expires_at: '2099-01-01T00:00:00Z' })
  context.lifecycle.show()
  expect(context.state.user).toBeNull()
  expect(context.state.profile.nickname).toBe('')
  context.respond(2, { ...user, nickname: '旧账号迟到响应' })
  context.respond(3, tenant)
  await flushPromises()
  expect(context.requests[4].header.Authorization).toBe('Bearer token-b')
  context.respond(4, { user: { ...user, id: '8', username: 'bob', nickname: '新账号' }, tenant })
  await flushPromises()
  expect(context.state.user.id).toBe('8')
  expect(context.wrapper.text()).toContain('新账号')
  expect(context.wrapper.text()).not.toContain('旧账号迟到响应')
  context.lifecycle.unload()
})

test('page onShow confirms an interrupted same-owner write through a fresh account read', async () => {
  const context = await setupPage()
  await click(context.wrapper, '编辑个人资料')
  await input(context.wrapper, '#nickname', '提交中的资料')
  await click(context.wrapper, '保存资料')
  expect(context.state.busy).toBe(true)
  context.lifecycle.show()
  expect(context.state.ready).toBe(false)
  expect(context.state.busy).toBe(false)
  context.respond(3, tenant)
  await flushPromises()
  context.respond(4, { user: { ...user, nickname: '服务端已保存' }, tenant })
  await flushPromises()
  context.respond(2, { ...user, nickname: '旧写入响应' })
  await flushPromises()
  expect(context.state.user.nickname).toBe('服务端已保存')
  expect(context.wrapper.text()).toContain('服务端已保存')
  expect(context.wrapper.text()).not.toContain('旧写入响应')
  context.lifecycle.unload()
})

test('WeChat page exchanges a fresh platform code and persists only the SaaS session', async () => {
  const context = await setupPage({ wechat: true, session: false })
  expect(context.wrapper.find('#username').exists()).toBe(false)
  await click(context.wrapper, '微信登录')
  expect(context.logins).toHaveLength(1)
  expect(context.logins[0]).toMatchObject({ provider: 'weixin', timeout: 10000 })
  expect(context.requests).toHaveLength(1)
  expect(context.state.busy).toBe(true)
  await click(context.wrapper, '微信登录')
  expect(context.logins).toHaveLength(1)
  context.logins[0].success({ code: 'temporary-code' })
  await flushPromises()
  expect(context.requests[1]).toMatchObject({
    url: 'https://acme.test/api/auth/wechat/login', method: 'POST', data: { code: 'temporary-code' },
    header: { 'Content-Type': 'application/json' },
  })
  expect(context.requests[1].header.Authorization).toBeUndefined()
  context.respond(1, { user: { ...user, username: 'wx_member', nickname: '微信用户' }, tenant, token: 'saas-token', expires_at: '2099-01-01T00:00:00Z' })
  await flushPromises()
  expect(context.wrapper.text()).toContain('微信用户')
  expect(context.wrapper.text()).toContain('编辑个人资料')
  expect(context.wrapper.text()).toContain('修改登录密码')
  expect(context.state.busy).toBe(false)
  expect(context.storage.get(context.storageKey)).toEqual({ token: 'saas-token', expires_at: '2099-01-01T00:00:00Z' })
  expect(JSON.stringify([...context.storage.values()])).not.toContain('temporary-code')
  context.lifecycle.unload()
})

test('WeChat login accepts the company contract and exposes password settings', async () => {
  const context = await setupPage({ wechat: true, session: false })
  expect(context.wrapper.text()).not.toContain('未开放')
  await click(context.wrapper, '微信登录')
  context.logins[0].success({ code: 'fresh-code' })
  await flushPromises()
  context.respond(1, { user, tenant, token: 'token-a', expires_at: '2099-01-01T00:00:00Z' })
  await flushPromises()
  await click(context.wrapper, '修改登录密码')
  expect(context.wrapper.find('#current-password').exists()).toBe(false)
  context.lifecycle.unload()
})

test.each([
  [503, 'wechat_not_configured', '本企业暂未配置微信登录，请联系企业管理员。'],
  [401, 'wechat_invalid_code', '微信登录凭证已失效，请重试。'],
  [429, 'wechat_rate_limited', '微信登录过于频繁，请稍后重试。'],
  [502, 'wechat_upstream_error', '微信登录服务暂时不可用，请稍后重试。'],
  [403, 'disabled', '企业或账号已停用'],
])('WeChat login failure %s/%s leaves a retryable login with no session', async (status, code, message) => {
  const context = await setupPage({ wechat: true, session: false })
  await click(context.wrapper, '微信登录')
  context.logins[0].success({ code: 'fresh-code' })
  await flushPromises()
  context.respond(1, { code, message }, status)
  await flushPromises()
  expect(context.state.user).toBeNull()
  expect(context.state.busy).toBe(false)
  expect(context.storage.has(context.storageKey)).toBe(false)
  expect(context.wrapper.get('[role="alert"]').text()).toContain(message)
  await click(context.wrapper, '微信登录')
  expect(context.logins).toHaveLength(2)
  context.lifecycle.unload()
})

test('unloading during platform login ignores its late code without creating an HTTP request', async () => {
  const context = await setupPage({ wechat: true, session: false })
  await click(context.wrapper, '微信登录')
  context.lifecycle.unload()
  context.logins[0].success({ code: 'late-code' })
  await flushPromises()
  expect(context.requests).toHaveLength(1)
  expect(context.state.user).toBeNull()
  expect(context.state.busy).toBe(false)
  expect(context.storage.has(context.storageKey)).toBe(false)
})

test('unloading after the platform returns a code prevents its queued HTTP submission', async () => {
  const context = await setupPage({ wechat: true, session: false })
  await click(context.wrapper, '微信登录')
  context.logins[0].success({ code: 'queued-code' })
  context.lifecycle.unload()
  await flushPromises()
  expect(context.requests).toHaveLength(1)
  expect(context.state.user).toBeNull()
  expect(context.storage.has(context.storageKey)).toBe(false)
})

test('page resume switches stored ownership while a WeChat code is pending', async () => {
  const context = await setupPage({ wechat: true, session: false })
  await click(context.wrapper, '微信登录')
  context.storage.set(context.storageKey, { token: 'token-b', expires_at: '2099-01-01T00:00:00Z' })
  context.lifecycle.show()
  context.logins[0].success({ code: 'old-owner-code' })
  await flushPromises()
  expect(context.requests.map(request => request.url)).not.toContain('https://acme.test/api/auth/wechat/login')
  context.respond(1, tenant)
  await flushPromises()
  expect(context.requests[2].header.Authorization).toBe('Bearer token-b')
  context.respond(2, { user: { ...user, id: '8', nickname: '新账号' }, tenant })
  await flushPromises()
  expect(context.state.user.id).toBe('8')
  expect(context.wrapper.text()).toContain('新账号')
  context.lifecycle.unload()
})

test('WeChat login rejects another enterprise response before persistence', async () => {
  const context = await setupPage({ wechat: true, session: false })
  await click(context.wrapper, '微信登录')
  context.logins[0].success({ code: 'fresh-code' })
  await flushPromises()
  context.respond(1, { user: { ...user, tenant_id: '3' }, tenant: { ...tenant, id: '3' }, token: 'wrong-tenant-token', expires_at: '2099-01-01T00:00:00Z' })
  await flushPromises()
  expect(context.state.user).toBeNull()
  expect(context.storage.has(context.storageKey)).toBe(false)
  expect(context.wrapper.get('[role="alert"]').text()).toContain('账户响应无效')
  context.lifecycle.unload()
})

test('WeChat login storage failure leaves no authorized view and permits a fresh attempt', async () => {
  const context = await setupPage({ wechat: true, session: false })
  context.uni.setStorageSync = () => { throw new Error('storage unavailable') }
  await click(context.wrapper, '微信登录')
  context.logins[0].success({ code: 'fresh-code' })
  await flushPromises()
  context.respond(1, { user, tenant, token: 'unpersisted-token', expires_at: '2099-01-01T00:00:00Z' })
  await flushPromises()
  expect(context.state.user).toBeNull()
  expect(context.state.busy).toBe(false)
  expect(context.storage.has(context.storageKey)).toBe(false)
  expect(context.wrapper.get('[role="alert"]').text()).toContain('无法保存本地登录信息')
  expect(context.wrapper.text()).not.toContain('编辑个人资料')
  await click(context.wrapper, '微信登录')
  expect(context.logins).toHaveLength(2)
  context.lifecycle.unload()
})


test('WeChat profile uses native nickname and avatar controls and saves the uploaded avatar', async () => {
  const context = await setupPage({ wechat: true })
  await click(context.wrapper, '编辑个人资料')
  expect(context.wrapper.get('#nickname').attributes('type')).toBe('nickname')
  expect(context.wrapper.find('#avatar-url').exists()).toBe(false)
  await input(context.wrapper, '#nickname', '微信昵称')
  await context.wrapper.get('[open-type="chooseAvatar"]').trigger('chooseavatar', { detail: { avatarUrl: '/tmp/wechat-avatar.jpg' } })
  await flushPromises()
  expect(context.uploads[0]).toMatchObject({ filePath: '/tmp/wechat-avatar.jpg', name: 'file', header: { Authorization: 'Bearer token-a' } })
  context.uploads[0].success({ statusCode: 200, data: JSON.stringify({ data: { avatar_url: '/api/avatars/2/7/avatar.jpg' } }) })
  await flushPromises()
  expect(context.state.profile.avatar_url).toBe('/api/avatars/2/7/avatar.jpg')
  await click(context.wrapper, '保存资料')
  expect(context.requests[2].data).toEqual({ nickname: '微信昵称', avatar_url: '/api/avatars/2/7/avatar.jpg' })
  context.respond(2, { ...user, nickname: '微信昵称', avatar_url: '/api/avatars/2/7/avatar.jpg' })
  await flushPromises()
  expect(context.wrapper.get('image').attributes('src')).toBe('https://acme.test/api/avatars/2/7/avatar.jpg')
  context.lifecycle.unload()
})

test('switching identity while an avatar upload is pending discards the upload result and draft', async () => {
  const context = await setupPage({ wechat: true })
  await click(context.wrapper, '编辑个人资料')
  await context.wrapper.get('[open-type="chooseAvatar"]').trigger('chooseavatar', { detail: { avatarUrl: '/tmp/old-avatar.jpg' } })
  await flushPromises()
  context.storage.set(context.storageKey, { token: 'token-b', expires_at: '2099-01-01T00:00:00Z' })
  context.lifecycle.show()
  context.uploads[0].success({ statusCode: 200, data: JSON.stringify({ data: { avatar_url: '/api/avatars/2/7/old.jpg' } }) })
  context.respond(2, tenant)
  await flushPromises()
  context.respond(3, { user: { ...user, id: '8', nickname: '新账号' }, tenant })
  await flushPromises()
  expect(context.state.user.id).toBe('8')
  expect(context.state.profile.avatar_url).toBe('')
  expect(context.wrapper.find('[open-type="chooseAvatar"]').exists()).toBe(false)
  context.lifecycle.unload()
})


test('an avatar response for another user is rejected without changing the profile draft', async () => {
  const context = await setupPage({ wechat: true })
  await click(context.wrapper, '编辑个人资料')
  await context.wrapper.get('[open-type="chooseAvatar"]').trigger('chooseavatar', { detail: { avatarUrl: '/tmp/avatar.png' } })
  await flushPromises()
  context.uploads[0].success({ statusCode: 200, data: JSON.stringify({ data: { avatar_url: '/api/avatars/2/8/other.png' } }) })
  await flushPromises()
  expect(context.state.profile.avatar_url).toBe('')
  expect(context.state.busy).toBe(false)
  expect(context.wrapper.get('[role="alert"]').text()).toContain('头像响应无效')
  context.lifecycle.unload()
})

test('native nickname submit reads the final cleared value rather than a previous input draft', async () => {
  const context = await setupPage({ wechat: true })
  await click(context.wrapper, '编辑个人资料')
  await input(context.wrapper, '#nickname', '先前输入')
  await context.wrapper.get('form').trigger('submit', { detail: { value: { nickname: '' } } })
  await flushPromises()
  expect(context.requests[2].data.nickname).toBe('')
  context.respond(2, { ...user, nickname: '' })
  await flushPromises()
  expect(context.state.user.nickname).toBe('')
  context.lifecycle.unload()
})


test('WeChat password settings verify a fresh code instead of asking for the unknown random password', async () => {
  const context = await setupPage({ wechat: true })
  await click(context.wrapper, '修改登录密码')
  expect(context.wrapper.find('#current-password').exists()).toBe(false)
  await input(context.wrapper, '#new-password', 'personalpassword1')
  await input(context.wrapper, '#confirm-password', 'personalpassword1')
  await click(context.wrapper, '确认修改密码')
  expect(context.logins).toHaveLength(1)
  expect(context.requests).toHaveLength(2)
  context.logins[0].success({ code: 'fresh-password-code' })
  await flushPromises()
  expect(context.requests[2]).toMatchObject({
    url: 'https://acme.test/api/account/wechat/password', method: 'POST',
    data: { code: 'fresh-password-code', new_password: 'personalpassword1' },
    header: { Authorization: 'Bearer token-a' },
  })
  context.respond(2, { ok: true })
  await flushPromises()
  expect(context.state.user).toBeNull()
  expect(context.state.passwords.new_password).toBe('')
  expect(context.storage.has(context.storageKey)).toBe(false)
  expect(context.wrapper.text()).toContain('微信登录')
  context.lifecycle.unload()
})

test('WeChat password verification failure preserves the form and a subsequent submission gets a new code', async () => {
  const context = await setupPage({ wechat: true })
  await click(context.wrapper, '修改登录密码')
  await input(context.wrapper, '#new-password', 'personalpassword1')
  await input(context.wrapper, '#confirm-password', 'personalpassword1')
  await click(context.wrapper, '确认修改密码')
  context.logins[0].success({ code: 'failed-password-code' })
  await flushPromises()
  context.respond(2, { code: 'wechat_login_blocked', message: '微信身份验证失败，请重试。' }, 403)
  await flushPromises()
  expect(context.state.user.id).toBe('7')
  expect(context.state.busy).toBe(false)
  expect(context.state.passwords.new_password).toBe('personalpassword1')
  await click(context.wrapper, '确认修改密码')
  expect(context.logins).toHaveLength(2)
  context.lifecycle.unload()
})

test('unloading while WeChat password code is pending prevents submission of a late verification', async () => {
  const context = await setupPage({ wechat: true })
  await click(context.wrapper, '修改登录密码')
  await input(context.wrapper, '#new-password', 'personalpassword1')
  await input(context.wrapper, '#confirm-password', 'personalpassword1')
  await click(context.wrapper, '确认修改密码')
  context.lifecycle.unload()
  context.logins[0].success({ code: 'late-password-code' })
  await flushPromises()
  expect(context.requests).toHaveLength(2)
  expect(context.storage.get(context.storageKey).token).toBe('token-a')
})
