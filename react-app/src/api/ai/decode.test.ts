import { expect, test } from 'vitest'
import { model, presets, provider } from './decode'
import { aiPresets, aiProviders } from '../../../tests/fixtures/ai'
import { embeddingModel } from '../../../tests/fixtures/knowledge'

test('模型解析完整 API Key，包括明确的空值', () => {
  expect(model(embeddingModel)).toEqual(embeddingModel)
  expect(model({ ...embeddingModel, api_key: '' }).api_key).toBe('')
})

test.each([undefined, null, true, 123, {}])('模型拒绝缺失或非字符串的 API Key：%j', api_key => {
  expect(() => model({ ...embeddingModel, api_key })).toThrow('服务返回的数据格式异常')
})

test('目录解码保留未知服务商的品牌和配置能力，不依赖前端品牌枚举', () => {
  const definition = { ...aiProviders[0], id: 'new-service', description: '新注册的服务', discovery: false, credential_required: false }
  expect(provider(definition)).toEqual(definition)
})

test.each(['https://external.test/logo.svg', 'javascript:alert(1)', 'data:text/html;base64,PHN2Zy8+'])('目录图形拒绝非内置图片地址：%s', logo => {
  expect(() => provider({ ...aiProviders[0], logo })).toThrow('服务返回的数据格式异常')
})

test('目录缺少能力声明时拒绝响应，避免静默显示错误的配置入口', () => {
  expect(() => provider({ ...aiProviders[0], discovery: undefined })).toThrow('服务返回的数据格式异常')
})

test('切分策略目录解码保留服务端新增策略元数据', () => {
  expect(presets({ ...aiPresets, chunk_strategies: [{ id: 'catalog-test', name: '服务端新增策略', description: '仅用于目录展示测试。' }] }).chunk_strategies).toEqual([
    { id: 'catalog-test', name: '服务端新增策略', description: '仅用于目录展示测试。' },
  ])
})

test.each([
  { chunk_strategies: undefined }, { chunk_strategies: null }, { chunk_strategies: {} },
  { chunk_strategies: [{ id: 'auto', name: '自动识别' }] },
])('缺失或不完整的切分策略目录必须报错：%j', ({ chunk_strategies }) => {
  expect(() => presets({ ...aiPresets, chunk_strategies })).toThrow('服务返回的数据格式异常')
})
