import { useState } from 'react'
import type { SessionInput } from '../../../api/chat/types'
import type { Presets } from '../../../api/ai/types'
import { ErrorNotice, Field, Modal } from '../../../components/ui'

export function ConversationSettings({ config, presets, onClose, onSaved, busy, error }: { config: SessionInput; presets: Presets; onClose: () => void; onSaved: (input: SessionInput) => void; busy: boolean; error: string }) {
  const [draft, setDraft] = useState(config)
  const update = <K extends keyof SessionInput>(key: K, value: SessionInput[K]) => setDraft(current => ({ ...current, [key]: value }))
  return <Modal title="当前对话设置" description="这些设置仅用于当前对话，保存后对后续提问生效。" onClose={onClose} busy={busy}>
    <form className="stacked-form modal-form" onSubmit={event => { event.preventDefault(); onSaved(draft) }}>
      <Field label="对话标题"><input aria-label="对话标题" required maxLength={100} value={draft.title} onChange={event => update('title', event.target.value)} /></Field>
      <Field label="提示词预设"><select aria-label="提示词预设" value="" onChange={event => { const item = presets.prompts.find(item => item.id === event.target.value); if (item) update('system_prompt', item.content) }}><option value="">选择预设以填入正文</option>{presets.prompts.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></Field>
      <Field label="当前对话 System Prompt"><textarea aria-label="当前对话 System Prompt" rows={6} value={draft.system_prompt} maxLength={16000} onChange={event => update('system_prompt', event.target.value)} /></Field>
      <div className="conversation-settings-row"><Field label="温度"><input aria-label="温度" type="number" min={0} max={2} step={0.1} value={draft.temperature} onChange={event => update('temperature', event.target.valueAsNumber)} /></Field><Field label="最大输出 Tokens"><input aria-label="最大输出 Tokens" type="number" min={1} max={32768} value={draft.max_tokens} onChange={event => update('max_tokens', event.target.valueAsNumber)} /></Field><Field label="历史轮数"><input aria-label="历史轮数" type="number" min={0} max={50} value={draft.history_turns} onChange={event => update('history_turns', event.target.valueAsNumber)} /></Field></div>
      <div className="conversation-settings-row"><Field label="检索策略"><select aria-label="检索策略" value={draft.retrieval_mode} onChange={event => { const value = event.target.value; if (value === 'keyword' || value === 'vector' || value === 'hybrid') update('retrieval_mode', value) }}><option value="hybrid">混合检索</option><option value="vector">语义向量</option><option value="keyword">关键词</option></select></Field><Field label="返回资料数量"><input aria-label="返回资料数量" type="number" min={1} max={20} value={draft.retrieval_limit} onChange={event => update('retrieval_limit', event.target.valueAsNumber)} /></Field><Field label="向量相似度阈值" hint="仅向量检索生效；0 表示不额外过滤。"><input aria-label="向量相似度阈值" type="number" min={0} max={1} step={0.01} value={draft.similarity_threshold} onChange={event => update('similarity_threshold', event.target.valueAsNumber)} /></Field></div>
      <ErrorNotice message={error} /><div className="modal-footer"><button type="button" className="button secondary" disabled={busy} onClick={onClose}>取消</button><button className="button primary" disabled={busy}>{busy ? '正在保存…' : '应用到当前对话'}</button></div>
    </form>
  </Modal>
}
