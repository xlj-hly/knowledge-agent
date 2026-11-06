import { afterEach, expect, test, vi } from 'vitest'
import { createWechatLogin } from './wechat.mjs'

afterEach(() => vi.useRealTimers())

function platform() {
  const calls = []
  return { calls, login(options) { calls.push(options) } }
}

test('gets a bounded fresh WeChat code without persisting or exposing other platform fields', async () => {
  const uni = platform()
  const login = createWechatLogin(uni)
  const pending = login.getCode()
  expect(uni.calls[0]).toMatchObject({ provider: 'weixin', timeout: 10000 })
  uni.calls[0].success({ code: 'temporary-code_123', openid: 'untrusted-field', session_key: 'untrusted-field' })
  await expect(pending).resolves.toBe('temporary-code_123')
})

test.each([undefined, {}, { code: '' }, { code: 'code with spaces' }, { code: '中文' }, { code: 'a'.repeat(257) }])('rejects malformed platform code %j', async response => {
  const uni = platform()
  const login = createWechatLogin(uni)
  const pending = login.getCode()
  uni.calls[0].success(response)
  await expect(pending).rejects.toMatchObject({ code: 'WECHAT_INVALID_CODE' })
})

test('platform failure and synchronous exceptions settle with retryable messages', async () => {
  const uni = platform()
  const login = createWechatLogin(uni)
  const pending = login.getCode()
  uni.calls[0].fail({ errMsg: 'private platform detail' })
  await expect(pending).rejects.toMatchObject({ code: 'WECHAT_LOGIN_FAILED', message: '无法完成微信登录，请重试。' })
  const unavailable = createWechatLogin({ login() { throw new Error('private platform detail') } })
  await expect(unavailable.getCode()).rejects.toMatchObject({ code: 'WECHAT_LOGIN_FAILED', message: '微信登录暂时不可用，请重试。' })
})

test('missing platform callback times out and a late callback cannot replace a newer attempt', async () => {
  vi.useFakeTimers()
  const uni = platform()
  const login = createWechatLogin(uni)
  const first = login.getCode()
  const rejection = expect(first).rejects.toMatchObject({ code: 'WECHAT_LOGIN_TIMEOUT' })
  await vi.advanceTimersByTimeAsync(10000)
  await rejection
  const second = login.getCode()
  uni.calls[0].success({ code: 'expired-code' })
  uni.calls[1].success({ code: 'fresh-code' })
  await expect(second).resolves.toBe('fresh-code')
  expect(vi.getTimerCount()).toBe(0)
})

test('cancellation settles immediately, clears its timer and ignores late callbacks', async () => {
  vi.useFakeTimers()
  const uni = platform()
  const login = createWechatLogin(uni)
  const pending = login.getCode()
  login.cancel()
  uni.calls[0].success({ code: 'late-code' })
  await expect(pending).rejects.toMatchObject({ code: 'CANCELLED' })
  expect(vi.getTimerCount()).toBe(0)
})

test('starting a new platform attempt cancels ownership of the previous request', async () => {
  const uni = platform()
  const login = createWechatLogin(uni)
  const first = login.getCode()
  const second = login.getCode()
  await expect(first).rejects.toMatchObject({ code: 'CANCELLED' })
  uni.calls[0].fail({ errMsg: 'late failure' })
  uni.calls[1].success({ code: 'new-code' })
  await expect(second).resolves.toBe('new-code')
})
