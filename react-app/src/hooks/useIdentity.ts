import { useCallback, useState } from 'react'
import type { Api } from '../api/client'
import type { Identity } from '../stores/session/types'
import { useResource } from './async'

/** 每次进入表单重新确认服务端状态，涵盖响应丢失但已提交的写入。 */
export function useIdentity(
  api: Api,
  platform: boolean,
  onChanged: (identity: Identity) => void,
) {
  const load = useCallback(
    async (signal: AbortSignal) => {
      const identity = await (platform ? api.platformMe(signal) : api.me(signal))
      if (!signal.aborted) onChanged(identity)
      return identity
    },
    [api, platform, onChanged],
  )
  const resource = useResource(load)
  const [updated, setUpdated] = useState<Identity | null>(null)
  const update = (next: Identity) => {
    setUpdated(next)
    onChanged(next)
  }
  return { ...resource, current: updated ?? resource.data, update }
}
