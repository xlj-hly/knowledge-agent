import { id, integer, invalid, record, string } from '../decode'
import { number } from '../ai/decode'
import { indexingMode, list } from '../knowledge/decode'
import type { ChatEvent, ChatSession, Message, Source } from './types'
export function session(value: unknown): ChatSession {
  const row = record(value)
  return { id: id(row.id), title: string(row.title), knowledge_base_ids: list(id)(row.knowledge_base_ids), system_prompt: string(row.system_prompt), temperature: number(row.temperature), max_tokens: integer(row.max_tokens, 1), history_turns: integer(row.history_turns), retrieval_mode: indexingMode(row.retrieval_mode), retrieval_limit: integer(row.retrieval_limit, 1), similarity_threshold: number(row.similarity_threshold), created_at: string(row.created_at), updated_at: string(row.updated_at) }
}
export function source(value: unknown): Source {
  const row = record(value)
  return { base_id: id(row.base_id), document_id: id(row.document_id), chunk_id: id(row.chunk_id), document_title: string(row.document_title), content: string(row.content), score: number(row.score) }
}
export function message(value: unknown): Message {
  const row = record(value)
  const role = row.role === 'user' || row.role === 'assistant' ? row.role : invalid()
  const status = row.status === 'generating' || row.status === 'completed' || row.status === 'failed' || row.status === 'cancelled' ? row.status : invalid()
  return { id: id(row.id), session_id: id(row.session_id), role, status, content: string(row.content), sources: list(source)(row.sources), created_at: string(row.created_at) }
}
export function event(type: string, value: unknown): ChatEvent {
  if (type === 'message' || type === 'done') return { type, message: message(value) }
  if (type === 'sources') return { type, sources: list(source)(value) }
  const row = record(value)
  if (type === 'delta') return { type, content: string(row.content) }
  if (type === 'error') return { type, code: string(row.code), message: string(row.message) }
  return invalid()
}
