import { useCallback, useState } from 'react'
import type { Api } from '../../../api/client'
import type { KnowledgeBase, KnowledgeDocument } from '../../../api/knowledge/types'
import { ErrorNotice, Field, Loading, Modal } from '../../../components/ui'
import { useAction, useResource } from '../../../hooks/async'

export function DocumentImport({ api, base, document, onClose, onSaved }: {
  api: Api; base: KnowledgeBase; document?: KnowledgeDocument; onClose: () => void; onSaved: () => void
}) {
  const content = useResource(useCallback(async (signal: AbortSignal) => document ? api.knowledgeDocument(document.id, signal) : null, [api, document]))
  if (document && content.loading) return <Modal title="编辑内容" description="正在读取资料原文。" onClose={onClose}><Loading /></Modal>
  if (document && content.error) return <Modal title="编辑内容" description="读取失败，请重试。" onClose={onClose}><ErrorNotice message={content.error} retry={content.refresh} /></Modal>
  return <ImportForm key={document?.id ?? 'new'} api={api} base={base} document={document} initialContent={content.data?.content ?? ''} onClose={onClose} onSaved={onSaved} />
}

function ImportForm({ api, base, document, initialContent, onClose, onSaved }: {
  api: Api; base: KnowledgeBase; document?: KnowledgeDocument; initialContent: string; onClose: () => void; onSaved: () => void
}) {
  const [mode, setMode] = useState<'file' | 'manual'>(document ? 'manual' : 'file')
  const [title, setTitle] = useState(document?.title ?? '')
  const [content, setContent] = useState(initialContent)
  const [file, setFile] = useState<File | null>(null)
  const [validation, setValidation] = useState('')
  const action = useAction(document?.id)
  return <Modal title={document ? '编辑文档内容' : '添加文档'} description="保存后自动解析、切分并建立索引，可在文档列表查看处理进度。" onClose={onClose} busy={action.busy}>
    {!document && <div className="knowledge-tabs knowledge-import-tabs" role="tablist" aria-label="添加方式"><button role="tab" aria-selected={mode === 'file'} disabled={action.busy} onClick={() => setMode('file')}>上传文件</button><button role="tab" aria-selected={mode === 'manual'} disabled={action.busy} onClick={() => setMode('manual')}>手动输入</button></div>}
    <form className="stacked-form modal-form knowledge-form" onSubmit={event => {
      event.preventDefault()
      if (mode === 'file') {
        if (!file) { setValidation('请选择要上传的文件'); return }
        if (file.size > 10 * 1024 * 1024) { setValidation('单个文件不能超过 10 MiB'); return }
        if (!/\.(txt|md|csv|json|pdf|docx)$/i.test(file.name)) { setValidation('请选择 TXT、Markdown、CSV、JSON、PDF 或 DOCX 文件'); return }
        if (/\.pdf$/i.test(file.name) && !base.parser_endpoint) { setValidation('请先在知识库设置中配置 PDF 解析服务地址'); return }
        setValidation('')
        void action.run(signal => api.uploadKnowledgeDocument(base.id, file, signal), onSaved)
      } else {
        if (!title.trim() || !content.trim()) { setValidation('请输入文档标题和正文'); return }
        setValidation('')
        void action.run(signal => api.saveKnowledgeDocument(base.id, { title: title.trim(), content }, document?.id, signal), onSaved)
      }
    }}>
      <fieldset disabled={action.busy}>{mode === 'file' ? <div className="knowledge-upload">
        <Field label="选择文件"><input aria-label="选择文件" type="file" accept=".txt,.md,.csv,.json,.pdf,.docx" onChange={event => setFile(event.target.files?.item(0) ?? null)} /></Field>
        <p>支持 TXT、Markdown、CSV、JSON、PDF、DOCX，单个文件不超过 10 MiB。文本与 DOCX 直接解析，PDF 使用知识库配置的 DocReader 服务。</p>
        {file && <p>已选择：{file.name} · {(file.size / 1024).toFixed(1)} KiB</p>}
      </div> : <><Field label="文档标题"><input aria-label="文档标题" value={title} maxLength={200} onChange={event => setTitle(event.target.value)} /></Field><Field label="正文内容"><textarea aria-label="正文内容" rows={12} value={content} onChange={event => setContent(event.target.value)} placeholder="输入或粘贴需要检索的资料…" /></Field>
        {document && <p className="knowledge-hint">保存会从更新后的正文重新切分，覆盖已有分块编辑。</p>}</>}
      </fieldset>
      <ErrorNotice message={validation || action.error} /><div className="modal-footer"><button type="button" className="button secondary" disabled={action.busy} onClick={onClose}>取消</button><button className="button primary" disabled={action.busy}>{action.busy ? '正在提交…' : document ? '保存并重新处理' : mode === 'file' ? '上传并处理' : '添加并处理'}</button></div>
    </form>
  </Modal>
}
