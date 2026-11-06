import { useMemo } from 'react'
import { useStore } from 'zustand'
import { createApi } from '../../api/client'
import { createSessionStore } from './store'

/** Store 按身份空间独立创建，浏览器持久化只用于刷新恢复；旧响应只能失效其原会话版本。 */
export function useSession({ storageKey }: { storageKey: string }) {
  const store = useMemo(() => createSessionStore(storageKey), [storageKey])
  const session = useStore(store)
  const { token, version, notice, storageNotice, changeToken, setNotice } = session
  const api = useMemo(
    () => createApi({ token, onUnauthorized: () => store.getState().expire(version) }),
    [token, version, store],
  )
  return { token, version, notice, storageNotice, api, changeToken, setNotice }
}
