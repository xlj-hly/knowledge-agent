import type { AISettings, Model, Presets, Provider } from '../../src/api/ai/types'
import { embeddingModel, vectorProviders } from './knowledge'

export const aiSettings: AISettings = {
  chat_model_id: '', embedding_model_id: '', vector_store_id: '', system_prompt: '请准确回答问题。',
  temperature: 0.7, max_tokens: 2048, history_turns: 5, retrieval_mode: 'hybrid', retrieval_limit: 5,
  similarity_threshold: 0, chunk_strategy: 'auto', chunk_size: 512, chunk_overlap: 80, parser_endpoint: '',
}
export const aiPresets: Presets = {
  prompts: [{ id: 'assistant', name: '通用助手', content: '你是一个严谨的企业助手。' }],
  chunking: [{ id: 'general', name: '通用资料', strategy: 'recursive', size: 1024, overlap: 100 }],
  chunk_strategies: [
    { id: 'auto', name: '自动识别', description: '根据资料结构选择切分方式。' },
    { id: 'recursive', name: '递归切分', description: '根据段落与标点逐步切分。' },
    { id: 'heading', name: '按标题结构', description: '沿用标题层级切分并保留上下文。' },
  ],
}
export const aiProviders: Provider[] = [{
  id: 'openai', name: 'OpenAI', protocol: 'openai', endpoint: 'https://api.openai.com/v1', kinds: ['chat', 'embedding'],
  description: 'OpenAI 模型服务', logo: 'data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciLz4=', credential_required: true, discovery: true,
  models: [{ id: 'text-embedding-3-small', name: 'text-embedding-3-small', kind: 'embedding', dimension: 1536 }],
}]
export const chatModel: Model = { ...embeddingModel, id: '92', name: '企业对话模型', kind: 'chat', model_name: 'gpt-test', dimension: 0 }

export function aiFixture(url: string): unknown {
  if (url.endsWith('/ai/settings')) return aiSettings
  if (url.endsWith('/ai/presets')) return aiPresets
  if (url.endsWith('/ai/providers')) return aiProviders
  if (url.endsWith('/knowledge/vector-providers')) return vectorProviders
  return []
}
