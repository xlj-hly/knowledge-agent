import { decodeOk, invalid } from '../decode'
import type { Request } from '../request'
import { base, chunk, chunkPreview, document, documentDetail, list, searchResult, vectorProvider, vectorStore } from './decode'
import type { IndexingMode, KnowledgeBaseInput, VectorStoreInput } from './types'

const root = '/knowledge'
const part = (value: string) => encodeURIComponent(value)
// 下载响应由请求层读取为 Blob；品牌检查同时接受浏览器其他 realm 的原生 Blob。
function isBlob(value: unknown): value is Blob { return Object.prototype.toString.call(value) === '[object Blob]' }
const baseInput = ({ name, description, indexing_mode, retrieval_mode, embedding_model_id, vector_store_id, parser_endpoint, chunk_size, chunk_overlap, chunk_strategy }: KnowledgeBaseInput) => ({ name, description, indexing_mode, retrieval_mode, embedding_model_id, vector_store_id, parser_endpoint, chunk_size, chunk_overlap, chunk_strategy })
export function createKnowledgeApi(request: Request) {
  return {
    knowledgeVectorProviders: (signal?: AbortSignal) => request(`${root}/vector-providers`, list(vectorProvider), 'GET', undefined, signal),
    knowledgeVectorStores: (signal?: AbortSignal) => request(`${root}/vector-stores`, list(vectorStore), 'GET', undefined, signal),
    saveKnowledgeVectorStore: (input: VectorStoreInput, id?: string, signal?: AbortSignal) => request(`${root}/vector-stores${id ? `/${part(id)}` : ''}`, vectorStore, id ? 'PATCH' : 'POST', input, signal),
    deleteKnowledgeVectorStore: (id: string, signal?: AbortSignal) => request(`${root}/vector-stores/${part(id)}`, decodeOk, 'DELETE', undefined, signal),
    testKnowledgeVectorStore: (id: string, signal?: AbortSignal) => request(`${root}/vector-stores/${part(id)}/test`, decodeOk, 'POST', undefined, signal),
    knowledgeBases: (signal?: AbortSignal) => request(`${root}/bases`, list(base), 'GET', undefined, signal),
    knowledgeBase: (id: string, signal?: AbortSignal) => request(`${root}/bases/${part(id)}`, base, 'GET', undefined, signal),
    saveKnowledgeBase: (input: KnowledgeBaseInput, id?: string, signal?: AbortSignal) => request(`${root}/bases${id ? `/${part(id)}` : ''}`, base, id ? 'PATCH' : 'POST', baseInput(input), signal),
    rebuildKnowledgeBase: (id: string, input: KnowledgeBaseInput, signal?: AbortSignal) => request(`${root}/bases/${part(id)}/rebuild`, base, 'POST', baseInput(input), signal),
    previewChunks: (input: { content: string; chunk_strategy: string; chunk_size: number; chunk_overlap: number }, signal?: AbortSignal) => request(`${root}/preview`, list(chunkPreview), 'POST', input, signal),
    deleteKnowledgeBase: (id: string, signal?: AbortSignal) => request(`${root}/bases/${part(id)}`, decodeOk, 'DELETE', undefined, signal),
    knowledgeDocuments: (id: string, signal?: AbortSignal) => request(`${root}/bases/${part(id)}/documents`, list(document), 'GET', undefined, signal),
    knowledgeDocument: (id: string, signal?: AbortSignal) => request(`${root}/documents/${part(id)}`, documentDetail, 'GET', undefined, signal),
    saveKnowledgeDocument: (baseId: string, input: { title: string; content: string }, id?: string, signal?: AbortSignal) => request(id ? `${root}/documents/${part(id)}` : `${root}/bases/${part(baseId)}/documents`, document, id ? 'PATCH' : 'POST', input, signal),
    uploadKnowledgeDocument: (id: string, file: File, signal?: AbortSignal) => {
      const body = new FormData()
      body.append('file', file)
      return request(`${root}/bases/${part(id)}/upload`, document, 'POST', body, signal)
    },
    deleteKnowledgeDocument: (id: string, signal?: AbortSignal) => request(`${root}/documents/${part(id)}`, decodeOk, 'DELETE', undefined, signal),
    reprocessKnowledgeDocument: (id: string, signal?: AbortSignal) => request(`${root}/documents/${part(id)}/reprocess`, document, 'POST', undefined, signal),
    downloadKnowledgeDocument: (id: string, signal?: AbortSignal) => request(`${root}/documents/${part(id)}/download`, value => isBlob(value) ? value : invalid(), 'GET', undefined, signal, 'blob'),
    knowledgeChunks: (id: string, signal?: AbortSignal) => request(`${root}/documents/${part(id)}/chunks`, list(chunk), 'GET', undefined, signal),
    updateKnowledgeChunk: (id: string, content: string, signal?: AbortSignal) => request(`${root}/chunks/${part(id)}`, document, 'PATCH', { content }, signal),
    searchKnowledge: (id: string, input: { query: string; mode: IndexingMode; limit: number }, signal?: AbortSignal) => request(`${root}/bases/${part(id)}/search`, list(searchResult), 'POST', input, signal),
  }
}
