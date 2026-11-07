import type { AISettings, AISettingsInput } from '../../api/ai/types'

export type SettingsGroup = 'models' | 'prompt' | 'generation' | 'retrieval' | 'chunking'

export const settingsGroups = [
  { id: 'models', title: '知识库默认连接', description: '为新知识库选择 Embedder 与向量存储。', icon: 'database' },
  { id: 'prompt', title: '系统提示词', description: '设定助手的角色、回答要求与表达方式。', icon: 'chat' },
  { id: 'generation', title: '生成参数', description: '控制回答长度、温度和历史上下文。', icon: 'spark' },
  { id: 'retrieval', title: '知识检索', description: '设置新对话的检索策略与资料数量。', icon: 'search' },
  { id: 'chunking', title: '文本分割与解析', description: '选择切分方案，预览资料处理效果。', icon: 'file' },
] as const

export const retrievalName = (mode: string) => ({ hybrid: '混合检索', vector: '语义向量', keyword: '关键词' })[mode] ?? mode

/** 每项仅提交所属字段，再由页面合并最近一次保存的完整模板。 */
export function groupPatch(group: SettingsGroup, draft: AISettings): Partial<AISettingsInput> {
  switch (group) {
    case 'models': return { embedding_model_id: draft.embedding_model_id, vector_store_id: draft.vector_store_id }
    case 'prompt': return { system_prompt: draft.system_prompt }
    case 'generation': return { temperature: draft.temperature, max_tokens: draft.max_tokens, history_turns: draft.history_turns }
    case 'retrieval': return { retrieval_mode: draft.retrieval_mode, retrieval_limit: draft.retrieval_limit, similarity_threshold: draft.similarity_threshold }
    case 'chunking': return { chunk_strategy: draft.chunk_strategy, chunk_size: draft.chunk_size, chunk_overlap: draft.chunk_overlap, parser_endpoint: draft.parser_endpoint }
  }
}
