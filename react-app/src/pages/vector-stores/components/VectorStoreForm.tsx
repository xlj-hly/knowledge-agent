import { useId, useState } from 'react'
import { Alert, Button, Checkbox, Drawer, Input, Space } from 'antd'
import type { Api } from '../../../api/client'
import type { VectorProvider, VectorStore } from '../../../api/knowledge/types'
import { ProviderLogo } from '../../../components/ProviderLogo'
import { useAction } from '../../../hooks/async'

function initialConfig(provider: VectorProvider, store?: VectorStore) {
  return Object.fromEntries(provider.fields.map(field => [field.key, store ? store.config[field.key] ?? '' : field.default]))
}

export function VectorStoreForm({ api, store, provider, onClose, onSaved }: { api: Api; store?: VectorStore; provider: VectorProvider; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(store?.name ?? '')
  const [config, setConfig] = useState(() => initialConfig(provider, store))
  const action = useAction(store?.id)
  const formId = useId()
  const update = (key: string, value: string) => setConfig(current => ({ ...current, [key]: value }))
  return <Drawer open title={store ? '编辑向量存储' : '添加向量存储'} size={520} onClose={onClose}
    closable={!action.busy} keyboard={!action.busy} mask={{ closable: !action.busy }}
    footer={<Space className="vector-editor-footer"><Button aria-label="取消" disabled={action.busy} onClick={onClose}>取消</Button><Button aria-label="保存连接" type="primary" htmlType="submit" form={formId} loading={action.busy}>保存连接</Button></Space>}>
    <form id={formId} className="vector-editor" onSubmit={event => {
      event.preventDefault()
      const submitted = Object.fromEntries(provider.fields.map(field => {
        const value = config[field.key] ?? ''
        return [field.key, field.secret ? value : value.trim()]
      }))
      void action.run(signal => api.saveKnowledgeVectorStore({ name: name.trim(), provider: provider.id, config: submitted }, store?.id, signal), onSaved)
    }}>
      <div className="vector-editor-brand"><ProviderLogo logo={provider.logo} size={36} /><div><strong>{provider.name}</strong><p>{provider.description}</p></div></div>
      <label className="vector-field"><span>连接名称</span><Input aria-label="连接名称" required maxLength={128} value={name} disabled={action.busy} onChange={event => setName(event.target.value)} placeholder="例如：企业知识索引" /></label>
      {store?.referenced && <Alert type="info" title="连接正在使用，索引归属参数已固定。" />}
      {provider.fields.map(field => {
        const disabled = action.busy || Boolean(store?.referenced && field.immutable)
        const common = { 'aria-label': field.label, disabled, required: field.required, value: config[field.key] ?? '' }
        const input = field.secret || field.type === 'password'
          ? <Input.Password {...common} autoComplete="new-password" placeholder={field.placeholder} onChange={event => update(field.key, event.target.value)} />
          : field.type === 'select'
            ? <select {...common} onChange={event => update(field.key, event.target.value)}>{!field.required && <option value="">未设置</option>}{field.options.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}</select>
            : field.type === 'boolean'
              ? <Checkbox aria-label={field.label} disabled={disabled} checked={config[field.key] === 'true'} onChange={event => update(field.key, String(event.target.checked))}>{field.label}</Checkbox>
              : <Input {...common} type={field.type === 'number' ? 'number' : 'text'} placeholder={field.placeholder} onChange={event => update(field.key, event.target.value)} />
        return <div className="vector-field" key={field.key}>
          {field.type === 'boolean' ? input : <label><span>{field.label}{field.required ? ' *' : ''}</span>{input}</label>}
          {field.help && <small>{field.help}</small>}
        </div>
      })}
      {action.error && <Alert type="error" showIcon role="alert" title={action.error} />}
    </form>
  </Drawer>
}
