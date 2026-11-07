import { ApiError } from './ApiError'
import { record, string } from './decode'

export interface ApiOptions {
  token?: string
  baseUrl?: string
  onUnauthorized?: () => void
  timeoutMs?: number
}
export type Request = <T>(
  path: string,
  decode: (value: unknown) => T,
  method?: string,
  body?: unknown,
  signal?: AbortSignal,
  responseType?: 'json' | 'blob',
) => Promise<T>

/** 请求期限包含正文读取；取消等待已发出的写入不会回滚服务端结果。 */
export function createRequest({
  token,
  baseUrl = '/api',
  onUnauthorized,
  timeoutMs = 15000,
}: ApiOptions): Request {
  if (!Number.isSafeInteger(timeoutMs) || timeoutMs <= 0)
    throw new Error('请求等待期限必须为正整数')
  return async (path, decode, method = 'GET', body, signal, responseType = 'json') => {
    const controller = new AbortController()
    let timedOut = false
    const cancel = () => controller.abort(signal?.reason)
    signal?.addEventListener('abort', cancel, { once: true })
    if (signal?.aborted) cancel()
    const timer = setTimeout(() => {
      timedOut = true
      controller.abort()
    }, timeoutMs)
    const headers: Record<string, string> = {
      Accept: responseType === 'blob' ? 'application/octet-stream' : 'application/json',
    }
    if (token) headers.Authorization = `Bearer ${token}`
    const multipart = body instanceof FormData
    // FormData 的 boundary 由浏览器生成，不能覆盖为 JSON 或手写 multipart 头。
    if (body !== undefined && !multipart) headers['Content-Type'] = 'application/json'
    try {
      const response = await fetch(`${baseUrl}${path}`, {
        method,
        headers,
        body: multipart ? body : body === undefined ? undefined : JSON.stringify(body),
        signal: controller.signal,
        cache: 'no-store',
        credentials: 'omit',
      })
      if (response.status === 401 && token && !controller.signal.aborted) onUnauthorized?.()
      if (response.ok && responseType === 'blob') return decode(await response.blob())
      let value: unknown
      try {
        value = await response.json()
      } catch (cause) {
        if (controller.signal.aborted) throw cause
        throw new ApiError('服务暂时不可用，请稍后重试', 'invalid_response', response.status)
      }
      const envelope = record(value)
      if (!response.ok) {
        const error = record(envelope.error)
        const code = string(error.code)
        if (response.status === 403 && code === 'disabled' && token && !controller.signal.aborted)
          onUnauthorized?.()
        throw new ApiError(string(error.message), code, response.status)
      }
      return decode(envelope.data)
    } catch (cause) {
      if (signal?.aborted) throw cause
      if (timedOut) throw new ApiError('服务响应超时，请重新加载以确认最新状态后重试', 'timeout')
      if (cause instanceof ApiError) throw cause
      throw new ApiError('无法连接服务，请检查网络后重试', 'network_error')
    } finally {
      clearTimeout(timer)
      signal?.removeEventListener('abort', cancel)
    }
  }
}
