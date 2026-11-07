import type { IndexingMode } from '../knowledge/types'
export interface SessionInput {
  title: string; knowledge_base_ids: string[]; system_prompt: string; temperature: number; max_tokens: number; history_turns: number; retrieval_mode: IndexingMode; retrieval_limit: number; similarity_threshold: number
}
export interface ChatSession extends SessionInput { id: string; created_at: string; updated_at: string }
export interface Source { base_id: string; document_id: string; chunk_id: string; document_title: string; content: string; score: number }
export interface Message { id: string; session_id: string; role: 'user' | 'assistant'; content: string; status: 'generating' | 'completed' | 'failed' | 'cancelled'; sources: Source[]; created_at: string }
export type ChatEvent = { type: 'message' | 'done'; message: Message } | { type: 'sources'; sources: Source[] } | { type: 'delta'; content: string } | { type: 'error'; code: string; message: string }
