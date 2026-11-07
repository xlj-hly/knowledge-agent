import type { IndexingMode } from '../knowledge/types'

export type ModelKind = 'chat' | 'embedding'
export interface ModelInput { name: string; kind: ModelKind; provider: string; endpoint: string; model_name: string; dimension: number; api_key: string }
export interface Model extends ModelInput { id: string; created_at: string; updated_at: string }
export interface ModelOption { id: string; name: string; kind: ModelKind; dimension: number }
export interface Provider {
  id: string; name: string; description: string; logo: string; protocol: string; endpoint: string
  kinds: ModelKind[]; models: ModelOption[]; credential_required: boolean; discovery: boolean
}
export interface AISettingsInput {
  embedding_model_id: string; vector_store_id: string
  system_prompt: string; temperature: number; max_tokens: number; history_turns: number
  retrieval_mode: IndexingMode; retrieval_limit: number; similarity_threshold: number
  chunk_strategy: string; chunk_size: number; chunk_overlap: number; parser_endpoint: string
}
export interface AISettings extends AISettingsInput { chat_model_id: string }
export interface Presets {
  prompts: { id: string; name: string; content: string }[]
  chunking: { id: string; name: string; strategy: string; size: number; overlap: number }[]
  chunk_strategies: { id: string; name: string; description: string }[]
}
