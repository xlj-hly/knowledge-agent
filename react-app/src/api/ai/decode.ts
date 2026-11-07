import { bool, id, integer, invalid, providerLogo, record, string } from '../decode'
import { indexingMode, list } from '../knowledge/decode'
import type { AISettings, Model, ModelKind, ModelOption, Presets, Provider } from './types'

export const kind = (value: unknown): ModelKind => value === 'chat' || value === 'embedding' ? value : invalid()
export const number = (value: unknown): number => typeof value === 'number' && Number.isFinite(value) ? value : invalid()
const optionalId = (value: unknown) => value === '' ? '' : id(value)
export function modelOption(value: unknown): ModelOption {
  const row = record(value)
  return { id: string(row.id), name: string(row.name), kind: kind(row.kind), dimension: integer(row.dimension) }
}
export function model(value: unknown): Model {
  const row = record(value)
  return { id: id(row.id), name: string(row.name), kind: kind(row.kind), provider: string(row.provider), endpoint: string(row.endpoint), model_name: string(row.model_name), dimension: integer(row.dimension), api_key: string(row.api_key), created_at: string(row.created_at), updated_at: string(row.updated_at) }
}
export function provider(value: unknown): Provider {
  const row = record(value)
  return { id: string(row.id), name: string(row.name), description: string(row.description), logo: providerLogo(row.logo), protocol: string(row.protocol), endpoint: string(row.endpoint), kinds: list(kind)(row.kinds), models: list(modelOption)(row.models), credential_required: bool(row.credential_required), discovery: bool(row.discovery) }
}
export function settings(value: unknown): AISettings {
  const row = record(value)
  return { chat_model_id: optionalId(row.chat_model_id), embedding_model_id: optionalId(row.embedding_model_id), vector_store_id: optionalId(row.vector_store_id), system_prompt: string(row.system_prompt), temperature: number(row.temperature), max_tokens: integer(row.max_tokens, 1), history_turns: integer(row.history_turns), retrieval_mode: indexingMode(row.retrieval_mode), retrieval_limit: integer(row.retrieval_limit, 1), similarity_threshold: number(row.similarity_threshold), chunk_strategy: string(row.chunk_strategy), chunk_size: integer(row.chunk_size, 1), chunk_overlap: integer(row.chunk_overlap), parser_endpoint: string(row.parser_endpoint) }
}
export function presets(value: unknown): Presets {
  const row = record(value)
  return {
    prompts: list(value => { const item = record(value); return { id: string(item.id), name: string(item.name), content: string(item.content) } })(row.prompts),
    chunking: list(value => { const item = record(value); return { id: string(item.id), name: string(item.name), strategy: string(item.strategy), size: integer(item.size, 1), overlap: integer(item.overlap) } })(row.chunking),
    chunk_strategies: list(value => { const item = record(value); return { id: string(item.id), name: string(item.name), description: string(item.description) } })(row.chunk_strategies),
  }
}
