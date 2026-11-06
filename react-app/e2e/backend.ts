import { setTimeout as delay } from 'node:timers/promises'

export interface BackendIdentityOptions {
  timeoutMs?: number
  pollMs?: number
  requestTimeoutMs?: number
  signal?: AbortSignal
}

/** 端口可访问不能证明实例归属；握手确认本次运行后才能开始测试写入。 */
export async function waitForBackendIdentity(baseUrl: string, runId: string, options: BackendIdentityOptions = {}): Promise<void> {
  const deadline = Date.now() + (options.timeoutMs ?? 60_000)
  while (Date.now() < deadline) {
    options.signal?.throwIfAborted()
    const timeout = AbortSignal.timeout(Math.max(1, Math.min(options.requestTimeoutMs ?? 1_000, deadline - Date.now())))
    const signal = options.signal ? AbortSignal.any([timeout, options.signal]) : timeout
    let response: Response | undefined
    try { response = await fetch(`${baseUrl}/__test/identity`, { signal }) } catch { options.signal?.throwIfAborted() }
    if (response) {
      if (!response.ok) throw new Error('E2E port is occupied by another instance (missing identity handshake)')
      let value: unknown
      try { value = await response.json() } catch { throw new Error('E2E port is occupied by another instance (invalid identity handshake)') }
      if (typeof value !== 'object' || value === null || !('run_id' in value) || value.run_id !== runId) throw new Error('E2E port is occupied by another instance (run ID mismatch)')
      return
    }
    const remaining = deadline - Date.now()
    if (remaining > 0) await delay(Math.min(options.pollMs ?? 200, remaining), undefined, { signal: options.signal })
  }
  throw new Error('E2E backend identity handshake timed out')
}

/** 等待清理完成；超时、信号终止或非零退出都会使测试失败。 */
export async function waitForBackendExit(exited: Promise<number | null>, timeoutMs: number): Promise<void> {
  let timer: ReturnType<typeof setTimeout> | undefined
  try {
    const code = await Promise.race([
      exited,
      new Promise<never>((_, reject) => { timer = setTimeout(() => reject(new Error('E2E backend cleanup timed out')), timeoutMs) }),
    ])
    if (code !== 0) throw new Error(`E2E backend cleanup failed (exit ${code})`)
  } finally { if (timer) clearTimeout(timer) }
}

/** 再次核对运行归属后停止实例，调用方负责失败时回收自己持有的子进程。 */
export async function stopBackend(baseUrl: string, runId: string, exited: Promise<number | null>, timeoutMs = 20_000): Promise<void> {
  await waitForBackendIdentity(baseUrl, runId, { timeoutMs: 2_000 })
  const response = await fetch(`${baseUrl}/__test/stop`, { method: 'POST', headers: { 'X-E2E-Run-ID': runId }, signal: AbortSignal.timeout(2_000) })
  if (response.status !== 204) throw new Error(`E2E backend stop rejected (HTTP ${response.status})`)
  await waitForBackendExit(exited, timeoutMs)
}
