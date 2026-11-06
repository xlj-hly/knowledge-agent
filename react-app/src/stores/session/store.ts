import { createStore } from 'zustand/vanilla'
import { createJSONStorage, persist } from 'zustand/middleware'
import type { PersistStorage } from 'zustand/middleware'
import type { SessionState } from './types'

interface PersistedSession {
  token: string
}

const storageVersion = 0

function record(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

function persistedSession(value: unknown): value is PersistedSession {
  return record(value) && typeof value.token === 'string'
    // 空令牌表示匿名；非空值只允许 RFC 6750 b64token 字符，避免恢复无法写入请求头的缓存。
    && value.token.length <= 256 && (value.token === '' || /^[-A-Za-z0-9._~+/]+=*$/.test(value.token))
}

/** 每个身份空间创建独立 Store；只恢复令牌，身份和权限由服务端重新确认。 */
export function createSessionStore(storageKey: string) {
  let storageNotice = ''
  // 延迟访问浏览器能力，存储被禁用时仍能构建和操作内存会话。
  const jsonStorage = createJSONStorage<unknown>(() => ({
    getItem: (name) => sessionStorage.getItem(name),
    setItem: (name, value) => sessionStorage.setItem(name, value),
    removeItem: (name) => sessionStorage.removeItem(name),
  }))
  if (!jsonStorage) throw new Error('无法创建会话存储')

  const storage: PersistStorage<PersistedSession> = {
    getItem(name) {
      try {
        // 当前存储为同步 sessionStorage；构建 Store 时只读，不迁移或改写旧记录。
        const saved: unknown = jsonStorage.getItem(name)
        if (saved === null) return null
        if (record(saved) && saved.version === storageVersion && persistedSession(saved.state)) {
          return { state: { token: saved.state.token }, version: storageVersion }
        }
        storageNotice = '本地登录信息无效，请重新登录。'
      } catch {
        storageNotice = '无法读取本地登录信息，请重新登录。'
      }
      return null
    },
    setItem(name, value) {
      try {
        // 退出及失效后的空令牌直接删除，后续提示更新也不能重新持久化旧身份。
        if (value.state.token) jsonStorage.setItem(name, value)
        else jsonStorage.removeItem(name)
      } catch {
        storageNotice = value.state.token
          ? '浏览器未允许保存会话，刷新页面后需要重新登录。'
          : '浏览器未允许清理会话，请检查存储权限。'
      }
    },
    removeItem(name) {
      try {
        jsonStorage.removeItem(name)
      } catch {
        storageNotice = '浏览器未允许清理会话，请检查存储权限。'
      }
    },
  }

  return createStore<SessionState>()(persist((set, get) => {
    const update = (changes: (state: SessionState) => Partial<SessionState>) => {
      storageNotice = ''
      // persist 先更新内存再写存储，版本推进不能被浏览器存储失败阻断。
      set((state) => ({ ...changes(state), storageNotice: '' }))
      // 在写入返回后报告失败，避免存储回调中更新 Store 形成递归写入。
      if (storageNotice) set({ storageNotice })
    }
    return {
      token: '',
      version: 0,
      notice: '',
      storageNotice: '',
      changeToken: (token) => update((state) => ({ token, version: state.version + 1 })),
      setNotice: (notice) => update(() => ({ notice })),
      expire(version) {
        const state = get()
        if (state.version !== version || !state.token) return
        state.changeToken('')
        get().setNotice('登录状态已失效，请重新登录。')
      },
    }
  }, {
    name: storageKey,
    version: storageVersion,
    storage,
    partialize: (state) => ({ token: state.token }),
    // 不使用默认浅合并：浏览器记录不能覆盖运行时版本、提示或操作函数。
    merge: (saved, current) => ({
      ...current,
      token: persistedSession(saved) ? saved.token : '',
      storageNotice,
    }),
  }))
}
