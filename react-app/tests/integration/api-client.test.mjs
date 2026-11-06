// @vitest-environment node
import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { test, afterEach } from 'vitest'
import { ApiError, createApi } from '../../src/api/client'

const tenant = {
  id: '9007199254740993',
  name: '测试企业',
  hostname: 'acme.test',
  status: 'active',
  registration_enabled: true,
  created_at: '2026-11-05T00:00:00Z',
  updated_at: '2026-11-05T00:00:00Z',
}
const user = {
  id: '2',
  tenant_id: tenant.id,
  username: 'alice',
  nickname: '小艾',
  avatar_url: '',
  role: 'admin',
  status: 'active',
  created_at: tenant.created_at,
  updated_at: tenant.updated_at,
}

async function serve(_t, handler) {
  const server = createServer(handler)
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve))
  afterEach(() => {
    server.closeAllConnections()
    return new Promise((resolve, reject) =>
      server.close((error) => (error ? reject(error) : resolve())),
    )
  })
  const address = server.address()
  assert.ok(address && typeof address === 'object')
  return `http://127.0.0.1:${address.port}/api`
}

test('企业登录由请求 Host 识别，不发送可伪造租户头；失效会话通知重新登录', async (t) => {
  let authRequests = 0
  const baseUrl = await serve(t, (request, response) => {
    response.setHeader('Content-Type', 'application/json')
    assert.equal(request.headers['x-tenant-hostname'], undefined)
    if (request.url === '/api/auth/login') {
      response.end(
        JSON.stringify({
          data: { token: 'test-session', expires_at: '2026-11-12T00:00:00Z', user, tenant },
        }),
      )
      return
    }
    assert.equal(request.headers.authorization, 'Bearer test-session')
    authRequests += 1
    if (authRequests === 1) {
      response.end(JSON.stringify({ data: { user, tenant } }))
    } else {
      response.writeHead(401)
      response.end(
        JSON.stringify({ error: { code: 'unauthorized', message: '登录已失效，请重新登录' } }),
      )
    }
  })
  const login = await createApi({ baseUrl }).login({
    username: 'alice',
    password: 'example-password',
  })
  assert.equal(login.tenant.id, '9007199254740993')
  let expired = 0
  const api = createApi({
    baseUrl,
    token: login.token,
    onUnauthorized: () => {
      expired += 1
    },
  })
  assert.equal((await api.me()).user.nickname, '小艾')
  await assert.rejects(
    api.me(),
    (error) =>
      error instanceof ApiError &&
      error.status === 401 &&
      error.message === '登录已失效，请重新登录',
  )
  assert.equal(expired, 1)
})

test('拒绝数字 ID，避免大整数精度损失变成可操作企业记录', async (t) => {
  const baseUrl = await serve(t, (_request, response) => {
    response.end(
      JSON.stringify({
        data: { items: [{ ...tenant, id: 9007199254740992 }], total: 1, page: 1, page_size: 20 },
      }),
    )
  })
  await assert.rejects(
    createApi({ baseUrl, token: 'platform-token' }).tenants('', 1),
    (error) => error instanceof ApiError && error.code === 'invalid_response',
  )
})

test('登录失败保留服务端业务说明，网络异常给出可重试提示', async (t) => {
  const baseUrl = await serve(t, (_request, response) => {
    response.writeHead(403)
    response.end(
      JSON.stringify({ error: { code: 'registration_closed', message: '企业已关闭自主注册' } }),
    )
  })
  await assert.rejects(
    createApi({ baseUrl }).register({
      username: 'alice',
      password: 'example-password',
      nickname: '',
    }),
    (error) =>
      error instanceof ApiError &&
      error.code === 'registration_closed' &&
      error.message === '企业已关闭自主注册',
  )
  await assert.rejects(
    createApi({ baseUrl: 'http://127.0.0.1:1/api' }).site(),
    (error) => error instanceof ApiError && error.code === 'network_error',
  )
})

test('企业停用会话立即退出；普通权限不足保持当前登录', async (t) => {
  const baseUrl = await serve(t, (request, response) => {
    response.writeHead(403)
    const disabled = request.url === '/api/auth/me'
    response.end(
      JSON.stringify({
        error: {
          code: disabled ? 'disabled' : 'forbidden',
          message: disabled ? '企业或账号已停用' : '需要企业管理员权限',
        },
      }),
    )
  })
  let expired = 0
  const api = createApi({
    baseUrl,
    token: 'session',
    onUnauthorized: () => {
      expired += 1
    },
  })
  await assert.rejects(
    api.users('', 1),
    (error) => error instanceof ApiError && error.code === 'forbidden',
  )
  assert.equal(expired, 0)
  await assert.rejects(api.me(), (error) => error instanceof ApiError && error.code === 'disabled')
  assert.equal(expired, 1)
})

test('个人资料以 PUT 完整保存昵称和头像，再解码服务端确认的账号', async (t) => {
  let received
  const baseUrl = await serve(t, async (request, response) => {
    const chunks = []
    for await (const chunk of request) chunks.push(chunk)
    received = { method: request.method, path: request.url, body: JSON.parse(Buffer.concat(chunks).toString()) }
    response.end(JSON.stringify({ data: { ...user, ...received.body } }))
  })
  const saved = await createApi({ baseUrl, token: 'session' }).profile({ nickname: '新昵称', avatar_url: '' })
  assert.deepEqual(received, { method: 'PUT', path: '/api/account/profile', body: { nickname: '新昵称', avatar_url: '' } })
  assert.equal(saved.nickname, '新昵称')
  assert.equal(saved.avatar_url, '')
  assert.equal(saved.tenant_id, tenant.id)
})

test('响应正文停滞也会超时，保留登录并可重新读取确认最新状态', async (t) => {
  let requests = 0
  const baseUrl = await serve(t, (_request, response) => {
    requests += 1
    if (requests === 1) {
      response.writeHead(200, { 'Content-Type': 'application/json' })
      response.write('{')
    } else response.end(JSON.stringify({ data: { kind: 'tenant', hostname: tenant.hostname, tenant } }))
  })
  let expired = 0
  const api = createApi({
    baseUrl,
    token: 'session',
    timeoutMs: 100,
    onUnauthorized: () => {
      expired += 1
    },
  })
  await assert.rejects(
    api.site(),
    (error) =>
      error instanceof ApiError && error.code === 'timeout' && /确认最新状态/.test(error.message),
  )
  assert.equal(expired, 0)
  assert.equal((await api.site()).tenant.id, '9007199254740993')
})

test('主动取消与超时保留不同语义，已发送的写入不会被宣称回滚', async (t) => {
  const controller = new AbortController()
  let writes = 0
  const baseUrl = await serve(t, (_request, response) => {
    writes += 1
    controller.abort()
    response.end(JSON.stringify({ data: { ok: true } }))
  })
  await assert.rejects(
    createApi({ baseUrl }).password(
      { current_password: 'old-password', new_password: 'new-password' },
      controller.signal,
    ),
    (error) =>
      error instanceof Error && error.name === 'AbortError' && !(error instanceof ApiError),
  )
  assert.equal(writes, 1)
})
