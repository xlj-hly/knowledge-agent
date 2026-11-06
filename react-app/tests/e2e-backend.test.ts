// @vitest-environment node
import { afterEach, describe, expect, it } from 'vitest'
import { createServer } from 'node:http'
import type { IncomingMessage, ServerResponse } from 'node:http'
import { waitForBackendIdentity, stopBackend } from '../e2e/backend.ts'

const cleanups: Array<() => Promise<void>> = []
afterEach(async () => { for (const close of cleanups.splice(0)) await close() })

async function server(handle: (request: IncomingMessage, response: ServerResponse) => void) {
  const instance = createServer(handle)
  await new Promise<void>(resolve => instance.listen(0, '127.0.0.1', resolve))
  cleanups.push(() => new Promise<void>((resolve, reject) => {
    instance.closeAllConnections()
    instance.close(error => error ? reject(error) : resolve())
  }))
  const address = instance.address()
  if (!address || typeof address === 'string') throw new Error('Expected a TCP test address')
  return `http://127.0.0.1:${address.port}`
}

describe('真实 E2E 服务的归属与清理', () => {
  it('拒绝健康但属于其他运行的服务，停止请求不应发出', async () => {
    let stops = 0
    const url = await server((request, response) => {
      if (request.url === '/__test/stop') stops++
      response.setHeader('Content-Type', 'application/json')
      response.end(JSON.stringify({ run_id: 'another-run' }))
    })
    await expect(waitForBackendIdentity(url, 'this-run')).rejects.toThrow('another instance')
    await expect(stopBackend(url, 'this-run', Promise.resolve(0))).rejects.toThrow('another instance')
    expect(stops).toBe(0)
  })

  it('核对运行标识后携带凭据停止自有服务，并等待成功清理', async () => {
    let stopCredential = ''
    const url = await server((request, response) => {
      if (request.url === '/__test/identity') {
        response.setHeader('Content-Type', 'application/json')
        response.end(JSON.stringify({ run_id: 'owned-run' }))
      } else {
        stopCredential = String(request.headers['x-e2e-run-id'])
        response.writeHead(204).end()
      }
    })
    await waitForBackendIdentity(url, 'owned-run')
    await stopBackend(url, 'owned-run', Promise.resolve(0))
    expect(stopCredential).toBe('owned-run')
  })

  it('有界结束未响应的身份探测，而不是无限等待健康接口', async () => {
    const url = await server(() => { /* A listener that never completes the handshake. */ })
    await expect(waitForBackendIdentity(url, 'owned-run', { timeoutMs: 40, requestTimeoutMs: 10, pollMs: 1 })).rejects.toThrow('timed out')
  })

  it('清理退出超时或失败会使测试失败', async () => {
    const url = await server((request, response) => {
      if (request.url === '/__test/identity') response.end(JSON.stringify({ run_id: 'owned-run' }))
      else response.writeHead(204).end()
    })
    await expect(stopBackend(url, 'owned-run', Promise.resolve(1), 40)).rejects.toThrow('cleanup failed')
    await expect(stopBackend(url, 'owned-run', new Promise(() => {}), 20)).rejects.toThrow('timed out')
  })
})
