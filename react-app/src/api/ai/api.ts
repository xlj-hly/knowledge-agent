import { bool, decodeOk, integer, record } from '../decode'
import { list } from '../knowledge/decode'
import type { Request } from '../request'
import { model, modelOption, presets, provider, settings } from './decode'
import type { AISettingsInput, ModelInput } from './types'

/** 当前启用 LLM 只由启用操作修改，模板保存明确序列化可写字段。 */
function settingsInput(input: AISettingsInput): AISettingsInput {
  return {
    embedding_model_id: input.embedding_model_id, vector_store_id: input.vector_store_id,
    system_prompt: input.system_prompt, temperature: input.temperature, max_tokens: input.max_tokens,
    history_turns: input.history_turns, retrieval_mode: input.retrieval_mode, retrieval_limit: input.retrieval_limit,
    similarity_threshold: input.similarity_threshold, chunk_strategy: input.chunk_strategy,
    chunk_size: input.chunk_size, chunk_overlap: input.chunk_overlap, parser_endpoint: input.parser_endpoint,
  }
}

export function createAIApi(request: Request) {
  return {
    aiModels: (signal?: AbortSignal) => request('/ai/models', list(model), 'GET', undefined, signal),
    aiProviders: (signal?: AbortSignal) => request('/ai/providers', list(provider), 'GET', undefined, signal),
    aiPresets: (signal?: AbortSignal) => request('/ai/presets', presets, 'GET', undefined, signal),
    aiSettings: (signal?: AbortSignal) => request('/ai/settings', settings, 'GET', undefined, signal),
    saveAISettings: (input: AISettingsInput, signal?: AbortSignal) => request('/ai/settings', settings, 'PUT', settingsInput(input), signal),
    activateAIModel: (id: string, signal?: AbortSignal) => request(`/ai/models/${encodeURIComponent(id)}/activate`, settings, 'PUT', undefined, signal),
    saveAIModel: (input: ModelInput, id?: string, signal?: AbortSignal) => request(`/ai/models${id ? `/${encodeURIComponent(id)}` : ''}`, model, id ? 'PUT' : 'POST', input, signal),
    deleteAIModel: (id: string, signal?: AbortSignal) => request(`/ai/models/${encodeURIComponent(id)}`, decodeOk, 'DELETE', undefined, signal),
    discoverAIModels: (input: ModelInput, signal?: AbortSignal) => request('/ai/models/discover', value => list(modelOption)(record(value).models), 'POST', input, signal),
    testAIModel: (input: ModelInput, signal?: AbortSignal) => request('/ai/models/test', value => { const row = record(value); return { ok: bool(row.ok), dimension: integer(row.dimension) } }, 'POST', input, signal),
  }
}
