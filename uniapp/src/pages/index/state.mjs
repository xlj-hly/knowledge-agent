import { validateCredentials, validateProfile, validatePasswordChange } from './validation.mjs'
import { createAccountApi } from '../../api/account.mjs'

const credentials = () => ({ username: '', password: '', nickname: '' })
const profile = () => ({ nickname: '', avatar_url: '' })
const passwords = () => ({ current_password: '', new_password: '', confirmation: '' })

export function createAccountState() {
  return {
    tenant: null, user: null, screen: 'login', ready: false, loading: true,
    busy: false, error: '', notice: '', avatarFailed: false,
    credentials: credentials(), profile: profile(), passwords: passwords(),
  }
}

// 页面拥有状态容器和草稿；控制器不依赖 Vue 实例或平台生命周期，平台访问由请求适配处理。
export function createAccountController({ state, client, wechat = null }) {
  const api = createAccountApi(client)
  let generation = 0
  let owner = ''

  function resetDrafts() {
    state.credentials = credentials()
    state.profile = profile()
    state.passwords = passwords()
    state.avatarFailed = false
  }

  function resetIdentity() {
    owner = ''
    state.user = null
    state.screen = 'login'
    resetDrafts()
  }

  function cancelPending() {
    generation++
    client.cancelAll()
    wechat?.cancel()
    state.busy = false
    state.loading = false
  }

  function sessionExpired(error) {
    cancelPending()
    resetIdentity()
    state.ready = Boolean(state.tenant) && error?.code !== 'disabled'
    state.error = error?.code === 'disabled' ? error.message : ''
    state.notice = error?.code === 'disabled' ? '' : '登录已失效，请重新登录。'
  }

  function acceptAccount(account) {
    const current = client.getSession()
    // 持久化或响应处理期间也可能到期；失效通知清理后不能继续恢复授权视图。
    if (!current) throw Object.assign(new Error('登录已失效，请重新登录。'), { code: 'UNAUTHORIZED' })
    const nextOwner = `${current.token}:${account.tenant.id}:${account.user.id}`
    if (owner !== nextOwner) {
      resetDrafts()
      state.screen = 'home'
    }
    owner = nextOwner
    state.user = account.user
    state.tenant = account.tenant
    if (!['home', 'profile', 'password'].includes(state.screen)) state.screen = 'home'
  }

  function synchronizeIdentity() {
    const previous = client.getSession()
    const current = client.syncSession()
    const changed = previous?.token !== current?.token || (!current && Boolean(state.user))
    if (changed) resetIdentity()
    return { current, changed }
  }

  async function refresh() {
    cancelPending()
    state.error = ''
    let snapshot
    try {
      snapshot = synchronizeIdentity()
    } catch (error) {
      resetIdentity()
      state.ready = false
      state.error = error.message
      return
    }
    return refreshAccount(snapshot.current)
  }

  async function refreshAccount(current) {
    const operationGeneration = generation
    state.loading = true
    state.ready = false
    try {
      const tenant = await api.getTenant()
      if (operationGeneration !== generation) return
      if (state.tenant && state.tenant.id !== tenant.id) resetIdentity()
      state.tenant = tenant
      if (current) {
        const account = await api.getAccount(tenant)
        if (operationGeneration !== generation) return
        acceptAccount(account)
      } else {
        state.user = null
        state.screen = state.screen === 'register' && tenant.registration_enabled ? 'register' : 'login'
      }
      state.ready = true
    } catch (error) {
      if (operationGeneration !== generation || error.code === 'CANCELLED') return
      resetIdentity()
      state.error = error.message
    } finally {
      if (operationGeneration === generation) state.loading = false
    }
  }

  function resume() {
    let snapshot
    try {
      snapshot = synchronizeIdentity()
    } catch (error) {
      cancelPending()
      resetIdentity()
      state.ready = false
      state.error = error.message
      return
    }
    // 首次 onLoad 紧接 onShow 时复用同身份的初始化；每次恢复仍先核对存储身份。
    if (state.loading && !snapshot.changed) return
    // 取消等待无法证明服务端写入已回滚，恢复后读取账户确认最终状态。
    const interrupted = state.busy
    cancelPending()
    if (interrupted) {
      resetDrafts()
      state.screen = state.user ? 'home' : 'login'
    }
    state.error = ''
    return refreshAccount(snapshot.current)
  }

  function switchScreen(screen) {
    if (state.busy) return
    cancelPending()
    state.screen = screen
    state.error = ''
    state.notice = ''
    state.credentials.password = ''
    state.passwords = passwords()
  }

  async function perform(operation, apply) {
    if (state.busy || !state.ready) return
    cancelPending()
    const operationGeneration = generation
    state.busy = true
    state.error = ''
    state.notice = ''
    try {
      const result = await operation()
      if (operationGeneration === generation) apply(result)
    } catch (error) {
      if (operationGeneration === generation && error.code !== 'CANCELLED') state.error = error.message
    } finally {
      if (operationGeneration === generation) state.busy = false
    }
  }

  function submitCredentials() {
    if (state.busy || !state.ready) return
    const registering = state.screen === 'register'
    state.error = validateCredentials(state.credentials, registering)
    if (state.error) return
    if (registering && !state.tenant.registration_enabled) {
      state.error = '本企业已关闭注册，请联系企业管理员。'
      return
    }
    const data = {
      username: state.credentials.username.trim().toLowerCase(), password: state.credentials.password,
      ...(registering ? { nickname: state.credentials.nickname.trim() } : {}),
    }
    return perform(async () => {
      const operationGeneration = generation
      if (registering) {
        const tenant = await api.getTenant()
        if (operationGeneration !== generation) throw Object.assign(new Error('请求已取消'), { code: 'CANCELLED' })
        state.tenant = tenant
        if (!tenant.registration_enabled) throw new Error('本企业已关闭注册，请联系企业管理员。')
      }
      return registering ? api.register(data, state.tenant) : api.login(data, state.tenant)
    }, result => {
      client.saveSession(result)
      acceptAccount(result)
      state.notice = registering ? '账号创建成功，欢迎加入。' : ''
    })
  }

  function loginWithWechat() {
    if (!wechat) return
    return perform(async () => {
      const operationGeneration = generation
      const code = await wechat.getCode()
      // 取得 code 与提交请求之间也可能发生页面卸载或身份替换。
      if (operationGeneration !== generation) throw Object.assign(new Error('微信登录已取消'), { code: 'CANCELLED' })
      return api.wechatLogin(code, state.tenant)
    }, result => {
      client.saveSession(result)
      acceptAccount(result)
    })
  }

  function openProfile() {
    if (state.busy || !state.user) return
    switchScreen('profile')
    state.profile = { nickname: state.user.nickname || '', avatar_url: state.user.avatar_url || '' }
  }

  function uploadAvatar(filePath) {
    if (!state.user || state.busy || state.screen !== 'profile') return
    const identity = { user: state.user, tenant: state.tenant }
    return perform(() => api.uploadAvatar(filePath, identity), avatarURL => {
      state.profile = { ...state.profile, avatar_url: avatarURL }
      state.avatarFailed = false
      state.notice = '头像已上传，保存资料后生效。'
    })
  }

  function saveProfile() {
    if (!state.user || state.busy) return
    const data = { nickname: state.profile.nickname.trim(), avatar_url: state.profile.avatar_url.trim() }
    state.error = validateProfile(data)
    if (state.error) return
    return perform(() => api.updateProfile(data, { user: state.user, tenant: state.tenant }), user => {
      state.user = user
      state.profile = profile()
      state.avatarFailed = false
      state.screen = 'home'
      state.notice = '个人资料已保存。'
    })
  }

  function endSession(notice) {
    // 即使本地存储删除失败，授权视图和草稿也必须先失效。
    resetIdentity()
    state.notice = notice
    client.clearSession()
  }

  function changePassword() {
    if (!state.user || state.busy) return
    state.error = validatePasswordChange(state.passwords, Boolean(wechat))
    if (state.error) return
    const data = { current_password: state.passwords.current_password, new_password: state.passwords.new_password }
    return perform(async () => {
      if (!wechat) return api.changePassword(data)
      const operationGeneration = generation
      // 微信会话通过新的 code 验证本人，用户无需知道首次生成的随机密码。
      const code = await wechat.getCode()
      if (operationGeneration !== generation) throw Object.assign(new Error('密码修改已取消'), { code: 'CANCELLED' })
      return api.changeWechatPassword({ code, new_password: data.new_password })
    }, () => {
      endSession('密码已修改，所有设备已退出登录。请重新登录。')
    })
  }

  function logout() {
    if (!state.user) return
    return perform(() => api.logout(), () => endSession('已安全退出登录。'))
  }

  return {
    refresh, resume, cancelPending, sessionExpired, switchScreen, submitCredentials, loginWithWechat, openProfile, uploadAvatar, saveProfile, changePassword, logout,
    avatarURL(path) { return client.avatarURL(path) },
    profileError(message) { if (!state.busy) state.error = message },
    updateCredentials(value) { state.credentials = value },
    updateProfile(value) { state.profile = value },
    updatePasswords(value) { state.passwords = value },
    avatarError() { state.avatarFailed = true },
  }
}
