import { expect, test } from 'vitest'
import { decodeWechatConfiguration } from './decode'

test('小程序配置解析 AppID 与完整 AppSecret，未配置时保留明确空值', () => {
  const configuration = { app_id: 'wx1234567890abcdef', app_secret: 'fixture-wechat-secret' }
  expect(decodeWechatConfiguration(configuration)).toEqual(configuration)
  expect(decodeWechatConfiguration({ app_id: '', app_secret: '' })).toEqual({ app_id: '', app_secret: '' })
})

test.each([undefined, null, false, 1, {}])('拒绝缺失或非字符串的 AppSecret：%j', app_secret => {
  expect(() => decodeWechatConfiguration({ app_id: 'wx1234567890abcdef', app_secret })).toThrow('服务返回的数据格式异常')
})
