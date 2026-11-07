import { useId, useState } from 'react'
import { Alert, AutoComplete, Button, Drawer, Input, InputNumber, Select, Space } from 'antd'
import type { Api } from '../../../api/client'
import type { Model, ModelInput, ModelKind, ModelOption, Provider } from '../../../api/ai/types'
import { ProviderLogo } from '../../../components/ProviderLogo'
import { useAction } from '../../../hooks/async'

export function ModelForm({ api, model, kind, providers, initialProvider, onClose, onSaved }: {
  api: Api; model?: Model; kind: ModelKind; providers: Provider[]; initialProvider?: string; onClose: () => void; onSaved: () => void
}) {
  const choices = providers.filter(item => item.kinds.includes(kind))
  const initial = choices.find(item => item.id === (model?.provider ?? initialProvider)) ?? choices[0]
  const [name, setName] = useState(model?.name ?? '')
  const [provider, setProvider] = useState(model?.provider ?? initial?.id ?? '')
  const [endpoint, setEndpoint] = useState(model?.endpoint ?? initial?.endpoint ?? '')
  const [modelName, setModelName] = useState(model?.model_name ?? '')
  const [dimension, setDimension] = useState(model?.dimension ?? 0)
  const [secret, setSecret] = useState(model?.api_key ?? '')
  const [discovered, setDiscovered] = useState<ModelOption[]>([])
  const [notice, setNotice] = useState('')
  const [validation, setValidation] = useState('')
  const action = useAction(model?.id)
  const formId = useId()
  const current = choices.find(item => item.id === provider)
  const options = [...discovered, ...(current?.models ?? [])].filter((item, index, items) => item.kind === kind && items.findIndex(other => other.id === item.id) === index)
  const resetDiscovery = () => { setDiscovered([]); setNotice(''); setValidation('') }
  const selectModel = (value: string) => {
    setModelName(value)
    const candidate = options.find(item => item.id === value)
    if (candidate?.dimension) setDimension(candidate.dimension)
  }
  const input = (): ModelInput => ({
    name: name.trim(), kind, provider,
    endpoint: endpoint.trim(), model_name: modelName.trim(), dimension: kind === 'chat' ? 0 : dimension,
    api_key: secret,
  })
  return <Drawer open title={model ? '编辑模型' : '添加模型'} size={560} onClose={onClose}
    closable={!action.busy} keyboard={!action.busy} mask={{ closable: !action.busy }}
    classNames={{ body: 'model-editor-body' }}
    footer={<Space className="model-editor-footer"><Button aria-label="取消" disabled={action.busy} onClick={onClose}>取消</Button><Button aria-label="保存模型" type="primary" htmlType="submit" form={formId} disabled={action.busy}>保存模型</Button></Space>}>
    <form id={formId} className="model-editor" noValidate onSubmit={event => {
      event.preventDefault()
      if (!name.trim() || !endpoint.trim() || !modelName.trim() || kind === 'embedding' && (!Number.isInteger(dimension) || dimension < 1)) {
        setValidation('请填写配置名称、服务地址、模型名称；嵌入器请先检测或填写有效维度'); return
      }
      setValidation('')
      void action.run(signal => api.saveAIModel(input(), model?.id, signal), onSaved)
    }}>
      <div className="model-editor-provider"><ProviderLogo logo={current?.logo} size={36} /><div><strong>{current?.name ?? provider}</strong><p>{current?.description}</p></div></div>
      <label className="model-field"><span>服务商</span><Select aria-label="服务商" disabled={action.busy} value={provider} virtual={false}
        options={choices.map(item => ({ value: item.id, label: <span className="model-provider-option"><ProviderLogo logo={item.logo} size={20} />{item.name}</span> }))}
        onChange={value => { setProvider(value); setEndpoint(choices.find(item => item.id === value)?.endpoint ?? ''); setModelName(''); setDimension(0); resetDiscovery() }} /></label>
      <label className="model-field"><span>服务地址</span><Input aria-label="服务地址" value={endpoint} disabled={action.busy} type="url" placeholder="https://api.example.com/v1"
        onChange={event => { setEndpoint(event.target.value); resetDiscovery() }} /><small>已填入服务商默认地址，可改为自建服务或代理。</small></label>
      <label className="model-field"><span>API Key</span><Input.Password aria-label="API Key" autoComplete="new-password" value={secret} disabled={action.busy} onChange={event => setSecret(event.target.value)}
        placeholder={current?.credential_required ? '填写服务商 API Key' : '服务无需密钥时可留空'} /><small>{current?.credential_required ? '服务商默认需要凭据；使用无需认证的代理时可留空。' : '按实际服务的认证要求填写，无需认证时可留空。'}</small></label>
      <div className="model-field"><div className="model-field-heading"><label htmlFor={`${formId}-model`}>模型名称</label>{current?.discovery && <Button size="small" disabled={action.busy || !endpoint.trim()} onClick={() => {
        setNotice(''); void action.run(signal => api.discoverAIModels(input(), signal), items => { setDiscovered(items); setNotice(items.length ? `获取到 ${items.length} 个模型，可在下方选择或手动输入。` : '服务没有返回模型列表，可以手动输入模型名称。') })
      }}>获取模型列表</Button>}</div>
        <AutoComplete disabled={action.busy} value={modelName} options={options.map(item => ({ value: item.id, label: item.name }))}
          filterOption={(value, option) => Boolean(option?.value.toLowerCase().includes(value.toLowerCase()))} onChange={selectModel}>
          <Input id={`${formId}-model`} aria-label="模型名称" placeholder="选择预设或输入模型标识" />
        </AutoComplete><small>{current?.discovery ? '可选择推荐模型，也可获取服务上的模型或手动输入。' : '此服务商不提供模型列表，请选择预设或手动填写模型名称。'}</small></div>
      <div className={kind === 'embedding' ? 'model-editor-grid' : undefined}>
        <label className="model-field"><span>配置名称</span><Input aria-label="配置名称" maxLength={100} value={name} disabled={action.busy} onChange={event => setName(event.target.value)} placeholder="在列表中显示的名称" /></label>
        {kind === 'embedding' && <label className="model-field"><span>向量维度</span><InputNumber aria-label="向量维度" min={0} max={65536} value={dimension} disabled={action.busy} onChange={value => setDimension(value ?? 0)} /></label>}
      </div>
      <Button block disabled={action.busy || !endpoint.trim() || !modelName.trim()} onClick={() => {
        setNotice(''); void action.run(signal => api.testAIModel(input(), signal), result => { if (kind === 'embedding') setDimension(result.dimension); setNotice(kind === 'embedding' ? `实际调用成功，向量维度为 ${result.dimension}。` : '实际模型调用成功。') })
      }}>测试调用{kind === 'embedding' ? '并检测维度' : ''}</Button>
      {notice && <Alert type="success" showIcon title={notice} role="status" />}
      {(validation || action.error) && <Alert type="error" showIcon title={validation || action.error} role="alert" />}
    </form>
  </Drawer>
}
