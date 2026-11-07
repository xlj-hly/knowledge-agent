import { expect, test } from 'vitest'
import { vectorProvider, vectorStore } from './decode'
import { vectorProviders } from '../../../tests/fixtures/knowledge'

test.each([null, undefined, [], { endpoint: null }, { port: 5432 }])('拒绝非字符串配置对象 %j', config => {
  expect(() => vectorStore({ id: '71', name: '连接', provider: 'pgvector', config, referenced: false, created_at: '', updated_at: '' })).toThrow('服务返回的数据格式异常')
})
test.each([null, undefined, {}, [{ ...vectorProviders[0].fields[0], type: 'unknown' }]])('拒绝无效引擎字段 %j', fields => {
  expect(() => vectorProvider({ ...vectorProviders[0], fields })).toThrow('服务返回的数据格式异常')
})
test('连接解析完整配置，保留凭据和明确的空字符串', () => {
  const store = { id: '71', name: '连接', provider: 'custom', config: { database: 'agent', token: 'fixture-vector-key', password: '' }, referenced: true, created_at: '', updated_at: '' }
  expect(vectorStore(store)).toEqual(store)
})
