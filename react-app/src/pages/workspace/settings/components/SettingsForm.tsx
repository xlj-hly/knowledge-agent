import { useState } from 'react'
import type { FormEvent } from 'react'
import { useAction } from '../../../../hooks/async'
import { ErrorNotice, Field, Icon, SuccessNotice, PageHeader } from '../../../../components/ui'
import type { TenantsApi } from '../../../../api/tenants/api'
import type { Tenant } from '../../../../api/tenants/types'
import '../settings.css'
export function SettingsForm({
  api,
  tenant,
  onUpdated,
}: {
  api: TenantsApi
  tenant: Tenant
  onUpdated: (tenant: Tenant) => void
}) {
  const [name, setName] = useState(tenant.name)
  const [validation, setValidation] = useState('')
  const [success, setSuccess] = useState('')
  const action = useAction()
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSuccess('')
    if (!name.trim()) {
      setValidation('请输入企业名称')
      return
    }
    setValidation('')
    void action.run(
      (signal) =>
        api.updateTenant({ name: name.trim() }, signal),
      (next) => {
        onUpdated(next)
        setSuccess('企业设置已保存。')
      },
    )
  }
  return (
    <>
      <PageHeader
        eyebrow="WORKSPACE SETTINGS"
        title="企业设置"
        description="维护企业名称与基本信息。"
      />
      <div className="settings-layout">
        <section className="panel settings-panel">
          <div className="section-heading">
            <span className="section-icon">
              <Icon name="building" />
            </span>
            <div>
              <h2>企业基本信息</h2>
              <p>这些信息将展示给你的团队成员。</p>
            </div>
          </div>
          <form className="stacked-form" onSubmit={submit}>
            <fieldset disabled={action.busy}>
              <Field label="企业名称">
                <input value={name} onChange={(event) => setName(event.target.value)} required />
              </Field>
              <Field label="企业访问域名" hint="如需变更访问域名，请联系平台管理员。">
                <input value={tenant.hostname} disabled />
              </Field>
            </fieldset>
            <ErrorNotice message={validation || action.error} />
            <SuccessNotice message={success} />
            <div className="form-footer">
              <button className="button primary" type="submit" disabled={action.busy}>
                {action.busy ? '正在保存…' : '保存企业设置'}
              </button>
            </div>
          </form>
        </section>
        <aside className="settings-aside">
          <div className="settings-help">
            <Icon name="shield" size={25} />
            <h3>维护企业基本信息</h3>
            <p>企业名称展示给团队成员。如需调整访问域名，请联系平台管理员。</p>
            <hr />
            <h4>企业标识</h4>
            <span className="mono">{tenant.id}</span>
            <h4>创建时间</h4>
            <span>{new Date(tenant.created_at).toLocaleDateString('zh-CN')}</span>
          </div>
        </aside>
      </div>
    </>
  )
}
