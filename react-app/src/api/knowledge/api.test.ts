import { afterEach, expect, test, vi } from 'vitest'
import { createApi } from '../client'

afterEach(() => vi.unstubAllGlobals())
test('原件下载沿用认证请求，读取二进制且在失败时保留服务端原因', async () => {
  vi.stubGlobal('fetch', async (_url: string, options: RequestInit) => {
    expect(options.headers).toMatchObject({ Authorization: 'Bearer tenant-session', Accept: 'application/octet-stream' })
    return new Response('original document', { headers: { 'Content-Type': 'text/plain' } })
  })
  const api = createApi({ token: 'tenant-session' })
  expect(await (await api.downloadKnowledgeDocument('71')).text()).toBe('original document')
  vi.stubGlobal('fetch', async () => new Response(JSON.stringify({ error: { code: 'not_found', message: '原件不存在' } }), { status: 404 }))
  await expect(api.downloadKnowledgeDocument('71')).rejects.toThrow('原件不存在')
})
