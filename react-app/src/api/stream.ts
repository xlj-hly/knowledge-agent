import { ApiError } from './ApiError'
import { record, string } from './decode'
import type { ApiOptions } from './request'

export type StreamRequest = (path: string, body: unknown, onEvent: (event: string, value: unknown) => void, signal?: AbortSignal) => Promise<void>

/** 流式连接与普通请求共享身份契约；按无数据期限取消，页面卸载也会终止读取。 */
export function createStreamRequest({ token, baseUrl = '/api', onUnauthorized, timeoutMs = 150000 }: ApiOptions): StreamRequest {
  return async (path, body, onEvent, signal) => {
    const controller = new AbortController()
    const cancel = () => controller.abort(signal?.reason)
    signal?.addEventListener('abort', cancel, { once: true })
    if (signal?.aborted) cancel()
    let timedOut = false
    let timer: ReturnType<typeof setTimeout> | undefined
    const renew = () => { clearTimeout(timer); timer = setTimeout(() => { timedOut = true; controller.abort() }, timeoutMs) }
    renew()
    let reader: ReadableStreamDefaultReader<Uint8Array> | undefined
    try {
      const headers: Record<string, string> = { Accept: 'text/event-stream', 'Content-Type': 'application/json' }
      if (token) headers.Authorization = `Bearer ${token}`
      const response = await fetch(`${baseUrl}${path}`, { method: 'POST', headers, body: JSON.stringify(body), signal: controller.signal, cache: 'no-store', credentials: 'omit' })
      if (!response.ok) {
        const envelope = record(await response.json())
        const error = record(envelope.error)
        const code = string(error.code)
        if (token && (response.status === 401 || response.status === 403 && code === 'disabled')) onUnauthorized?.()
        throw new ApiError(string(error.message), code, response.status)
      }
      if (!response.body || !response.headers.get('Content-Type')?.includes('text/event-stream')) throw new ApiError('服务未返回有效的对话流', 'invalid_response')
      reader = response.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''
      const consume = () => {
        let boundary: number
        while ((boundary = buffer.indexOf('\n\n')) >= 0) {
          const block = buffer.slice(0, boundary)
          buffer = buffer.slice(boundary + 2)
          let event = 'message'
          const data: string[] = []
          for (const line of block.split('\n')) {
            if (line.startsWith('event:')) event = line.slice(6).trim()
            if (line.startsWith('data:')) data.push(line.slice(5).trimStart())
          }
          if (data.length) {
            let value: unknown
            try { value = JSON.parse(data.join('\n')) } catch { throw new ApiError('对话流格式异常，请刷新确认消息状态', 'invalid_response') }
            onEvent(event, value)
          }
        }
      }
      while (true) {
        const result = await reader.read()
        renew()
        buffer = (buffer + decoder.decode(result.value, { stream: !result.done })).replace(/\r\n/g, '\n')
        consume()
        if (buffer.length > 2 * 1024 * 1024) throw new ApiError('对话事件过大', 'invalid_response')
        if (result.done) break
      }
      if (buffer.trim()) throw new ApiError('对话连接提前结束，请刷新确认消息状态', 'incomplete_stream')
    } catch (error) {
      if (signal?.aborted) throw error
      if (timedOut) throw new ApiError('对话响应超时，请刷新确认已保存的消息', 'timeout')
      if (error instanceof ApiError) throw error
      throw new ApiError('对话连接中断，请刷新确认消息状态', 'network_error')
    } finally {
      clearTimeout(timer)
      await reader?.cancel().catch(() => {})
      reader?.releaseLock()
      signal?.removeEventListener('abort', cancel)
    }
  }
}
