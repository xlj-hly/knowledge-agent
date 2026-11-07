import { useCallback, useState } from 'react'
import type { FormEvent } from 'react'
import { Input } from 'antd'
import type { TenantsApi } from '../../../../api/tenants/api'
import type { Tenant, WechatConfiguration } from '../../../../api/tenants/types'
import { ErrorNotice, Field, Loading, Modal } from '../../../../components/ui'
import { useAction, useResource } from '../../../../hooks/async'

/** 表单和秘密草稿只属于当前租户；切换租户或身份时卸载并取消旧请求。 */
export function WechatForm({ api, tenant, onClose, onSaved }: {
  api: TenantsApi
  tenant: Tenant
  onClose: () => void
  onSaved: () => void
}) {
  const load = useCallback((signal: AbortSignal) => api.tenantWechat(tenant.id, signal), [api, tenant.id])
  const configuration = useResource(load)
  const action = useAction(tenant.id)
  return (
    <Modal title="小程序配置" description={`${tenant.name} · ${tenant.hostname}`} onClose={onClose} busy={action.busy}>
      {configuration.loading && <Loading label="正在读取小程序配置…" />}
      <ErrorNotice message={configuration.error} retry={configuration.refresh} />
      {configuration.data && (
        <ConfigurationFields key={tenant.id} api={api} tenantId={tenant.id} configuration={configuration.data} action={action} onClose={onClose} onSaved={onSaved} />
      )}
    </Modal>
  )
}

function ConfigurationFields({ api, tenantId, configuration, action, onClose, onSaved }: {
  api: TenantsApi
  tenantId: string
  configuration: WechatConfiguration
  action: ReturnType<typeof useAction>
  onClose: () => void
  onSaved: () => void
}) {
  const [appId, setAppId] = useState(configuration.app_id)
  const [secret, setSecret] = useState(configuration.app_secret)
  const [validation, setValidation] = useState('')
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const error = !/^wx[0-9a-fA-F]{16}$/.test(appId.trim())
      ? '请填写有效的小程序 AppID'
      : !secret.trim()
        ? '请填写小程序 AppSecret'
        : ''
    setValidation(error)
    if (error) return
    void action.run((signal) => api.saveTenantWechat(tenantId, {
      app_id: appId.trim(),
      app_secret: secret.trim(),
    }, signal), saved => {
      setAppId(saved.app_id)
      setSecret(saved.app_secret)
      onSaved()
    })
  }
  return (
    <form className="stacked-form modal-form" onSubmit={submit}>
      <fieldset disabled={action.busy}>
        <Field label="小程序 AppID">
          <input value={appId} onChange={(event) => setAppId(event.target.value)} maxLength={18} autoComplete="off" autoCapitalize="none" spellCheck={false} required />
        </Field>
        <Field label="小程序 AppSecret" hint="填写该 AppID 对应的密钥。">
          <Input.Password aria-label="小程序 AppSecret" value={secret} onChange={(event) => setSecret(event.target.value)} maxLength={256} autoComplete="new-password" />
        </Field>
      </fieldset>
      <ErrorNotice message={validation || action.error} />
      <div className="modal-footer">
        <button className="button secondary" type="button" onClick={onClose} disabled={action.busy}>取消</button>
        <button className="button primary" type="submit" disabled={action.busy}>{action.busy ? '正在保存…' : '保存小程序配置'}</button>
      </div>
    </form>
  )
}
