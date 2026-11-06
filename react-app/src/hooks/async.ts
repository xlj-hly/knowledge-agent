import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : '操作失败，请稍后重试'
}

export function useResource<T>(load: (signal: AbortSignal) => Promise<T>) {
  const [state, setState] = useState<{
    data: T | null
    error: string
    load: typeof load | null
    revision: number
  }>({ data: null, error: '', load: null, revision: -1 })
  const [revision, setRevision] = useState(0)
  const refresh = useCallback(() => setRevision((value) => value + 1), [])
  useEffect(() => {
    const controller = new AbortController()
    load(controller.signal).then(
      (data) => {
        if (!controller.signal.aborted) setState({ data, error: '', load, revision })
      },
      (error: unknown) => {
        if (!controller.signal.aborted)
          setState({ data: null, error: errorMessage(error), load, revision })
      },
    )
    return () => controller.abort()
  }, [load, revision])
  const loading = state.load !== load || state.revision !== revision
  return { data: loading ? null : state.data, error: loading ? '' : state.error, loading, refresh }
}

/** 卸载或操作所属身份变化时取消等待，旧回调不能启动写入或发布结果。 */
export function useAction(scope?: string) {
  const [state, setState] = useState({ scope, busy: false, error: '' })
  const identity = useMemo(() => ({ scope }), [scope])
  const lifecycle = useRef({ identity, active: false, running: null as AbortController | null })
  useEffect(() => {
    const owner = { identity, active: true, running: null as AbortController | null }
    lifecycle.current = owner
    return () => {
      owner.active = false
      owner.running?.abort()
      owner.running = null
    }
  }, [identity])
  const run = useCallback(
    async <T>(task: (signal: AbortSignal) => Promise<T>, onSuccess?: (result: T) => void) => {
      const owner = lifecycle.current
      if (owner.identity !== identity || !owner.active || owner.running) return
      const controller = new AbortController()
      owner.running = controller
      setState({ scope, busy: true, error: '' })
      try {
        const result = await task(controller.signal)
        if (owner.active && !controller.signal.aborted) onSuccess?.(result)
      } catch (cause) {
        if (owner.active && !controller.signal.aborted)
          setState({ scope, busy: false, error: errorMessage(cause) })
      } finally {
        if (owner.running === controller) {
          owner.running = null
          if (owner.active) setState((current) => ({ ...current, busy: false }))
        }
      }
    },
    [scope, identity],
  )
  return {
    busy: state.scope === scope && state.busy,
    error: state.scope === scope ? state.error : '',
    run,
  }
}
