export type IndexingMode = 'keyword' | 'vector' | 'hybrid'
export interface VectorProvider {
  id: string
  name: string
  description: string
  logo: string
  protocol: string
  fields: VectorField[]
}
export interface VectorField {
  key: string
  label: string
  type: 'text' | 'password' | 'number' | 'select' | 'boolean'
  required: boolean
  secret: boolean
  immutable: boolean
  default: string
  placeholder: string
  help: string
  options: { value: string; label: string }[]
}
export interface VectorStore {
  id: string
  name: string
  provider: string
  config: Record<string, string>
  referenced: boolean
  created_at: string
  updated_at: string
}
export interface VectorStoreInput {
  name: string
  provider: string
  config: Record<string, string>
}
export interface KnowledgeBaseInput {
  name: string
  description: string
  indexing_mode: IndexingMode
  retrieval_mode: IndexingMode
  embedding_model_id: string
  vector_store_id: string
  parser_endpoint: string
  chunk_size: number
  chunk_overlap: number
  chunk_strategy: string
}
export interface KnowledgeBase extends KnowledgeBaseInput {
  id: string
  document_count: number
  chunk_count: number
  processing_count: number
  created_at: string
  updated_at: string
}
export interface KnowledgeDocument {
  id: string
  base_id: string
  title: string
  source: 'manual' | 'file'
  file_name: string
  file_type: string
  file_size: number
  parse_status: 'queued' | 'processing' | 'completed' | 'failed' | 'deleting'
  error_message: string
  chunk_count: number
  created_at: string
  updated_at: string
}
export interface DocumentDetail extends KnowledgeDocument { content: string }
export interface Chunk {
  id: string
  document_id: string
  content: string
  context_header: string
  index: number
  start: number
  end: number
}
export interface ChunkPreview { content: string; context_header: string; index: number; start: number; end: number }
export interface SearchResult extends Chunk {
  document_title: string
  file_name: string
  score: number
}
