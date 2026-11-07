import type { Chunk, KnowledgeBase, KnowledgeDocument, VectorProvider } from '../../src/api/knowledge/types'
import type { Model } from '../../src/api/ai/types'

export const vectorProviders: VectorProvider[] = [{
  id: 'qdrant', name: 'Qdrant', description: '连接自建实例或 Qdrant Cloud · HTTP API', protocol: 'qdrant',
  logo: 'data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciLz4=',
  fields: [
    { key: 'endpoint', label: '服务地址', type: 'text', required: true, secret: false, immutable: true, default: 'http://host.docker.internal:6333', placeholder: '', help: '实例 HTTP(S) 地址。', options: [] },
    { key: 'api_key', label: 'API Key', type: 'password', required: false, secret: true, immutable: false, default: '', placeholder: '', help: '', options: [] },
    { key: 'collection', label: '集合名称', type: 'text', required: true, secret: false, immutable: true, default: 'agent_knowledge', placeholder: '', help: '本应用索引前缀。', options: [] },
  ],
}]

export const knowledgeBase: KnowledgeBase = {
  id: '42', name: '员工手册', description: '企业内部使用指南', indexing_mode: 'keyword', embedding_model_id: '', vector_store_id: '',
  parser_endpoint: '', retrieval_mode: 'keyword', chunk_strategy: 'auto', chunk_size: 512, chunk_overlap: 80, document_count: 1, chunk_count: 1, processing_count: 0,
  created_at: '2026-11-06T00:00:00Z', updated_at: '2026-11-06T00:00:00Z',
}
export const knowledgeDocument: KnowledgeDocument = {
  id: '71', base_id: '42', title: '休假制度', source: 'manual', file_name: '', file_type: '', file_size: 0,
  parse_status: 'completed', error_message: '', chunk_count: 1, created_at: '2026-11-06T00:00:00Z', updated_at: '2026-11-06T00:00:00Z',
}
export const knowledgeChunk: Chunk = { id: '81', document_id: '71', content: '年假申请需要提前三天提交。', context_header: '', index: 0, start: 0, end: 16 }
export const embeddingModel: Model = {
  id: '91', name: '企业模型', provider: 'openai', endpoint: 'http://embedding.test/v1', model_name: 'text-embedding', dimension: 1024,
  kind: 'embedding', api_key: 'fixture-model-key', created_at: '2026-11-06T00:00:00Z', updated_at: '2026-11-06T00:00:00Z',
}
