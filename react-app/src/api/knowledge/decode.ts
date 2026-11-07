import { bool, id, integer, invalid, providerLogo, record, string } from '../decode'
import type { Chunk, ChunkPreview, DocumentDetail, IndexingMode, KnowledgeBase, KnowledgeDocument, SearchResult, VectorField, VectorProvider, VectorStore } from './types'

function choice<T extends string>(value: unknown, choices: readonly T[]): T {
  for (const item of choices) if (value === item) return item
  return invalid()
}
function optionalId(value: unknown) { return value === '' ? '' : id(value) }
export const indexingMode = (value: unknown): IndexingMode => choice(value, ['keyword', 'vector', 'hybrid'])
export function list<T>(decode: (value: unknown) => T) {
  return (value: unknown): T[] => Array.isArray(value) ? value.map(decode) : invalid()
}
export function vectorStore(value: unknown): VectorStore {
  const row = record(value)
  return { id: id(row.id), name: string(row.name), provider: string(row.provider), config: stringMap(row.config),
    referenced: bool(row.referenced), created_at: string(row.created_at), updated_at: string(row.updated_at) }
}
function stringMap(value: unknown): Record<string, string> {
  return Object.fromEntries(Object.entries(record(value)).map(([key, item]) => [key, string(item)]))
}
function vectorField(value: unknown): VectorField {
  const row = record(value)
  return { key: string(row.key), label: string(row.label), type: choice(row.type, ['text', 'password', 'number', 'select', 'boolean']),
    required: bool(row.required), secret: bool(row.secret), immutable: bool(row.immutable), default: string(row.default),
    placeholder: string(row.placeholder), help: string(row.help), options: list(value => { const option = record(value); return { value: string(option.value), label: string(option.label) } })(row.options) }
}
export function vectorProvider(value: unknown): VectorProvider {
  const row = record(value)
  return { id: string(row.id), name: string(row.name), description: string(row.description), logo: providerLogo(row.logo), protocol: string(row.protocol), fields: list(vectorField)(row.fields) }
}
export function base(value: unknown): KnowledgeBase {
  const row = record(value)
  return { id: id(row.id), name: string(row.name), description: string(row.description), indexing_mode: indexingMode(row.indexing_mode),
    retrieval_mode: indexingMode(row.retrieval_mode), chunk_strategy: string(row.chunk_strategy),
    embedding_model_id: optionalId(row.embedding_model_id), vector_store_id: optionalId(row.vector_store_id),
    parser_endpoint: string(row.parser_endpoint), chunk_size: integer(row.chunk_size, 1), chunk_overlap: integer(row.chunk_overlap),
    document_count: integer(row.document_count), chunk_count: integer(row.chunk_count), processing_count: integer(row.processing_count), created_at: string(row.created_at), updated_at: string(row.updated_at) }
}
export function document(value: unknown): KnowledgeDocument {
  const row = record(value)
  return { id: id(row.id), base_id: id(row.base_id), title: string(row.title), source: choice(row.source, ['manual', 'file']),
    file_name: string(row.file_name), file_type: string(row.file_type), file_size: integer(row.file_size),
    parse_status: choice(row.parse_status, ['queued', 'processing', 'completed', 'failed', 'deleting']), error_message: string(row.error_message),
    chunk_count: integer(row.chunk_count), created_at: string(row.created_at), updated_at: string(row.updated_at) }
}
export function documentDetail(value: unknown): DocumentDetail { return { ...document(value), content: string(record(value).content) } }
export function chunk(value: unknown): Chunk {
  const row = record(value)
  return { id: id(row.id), document_id: id(row.document_id), ...chunkPreview(row) }
}
export function chunkPreview(value: unknown): ChunkPreview { const row = record(value); return { content: string(row.content), context_header: string(row.context_header), index: integer(row.index), start: integer(row.start), end: integer(row.end) } }
export function searchResult(value: unknown): SearchResult {
  const row = record(value)
  if (typeof row.score !== 'number' || !Number.isFinite(row.score)) return invalid()
  return { ...chunk(value), document_title: string(row.document_title), file_name: string(row.file_name), score: row.score }
}
