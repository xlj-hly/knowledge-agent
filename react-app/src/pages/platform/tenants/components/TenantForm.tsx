import { useState } from 'react'
import type { FormEvent } from 'react'
import { useAction } from '../../../../hooks/async'
import { ErrorNotice, Field, Icon, Modal } from '../../../../components/ui'
import { newPasswordInput, passwordError, usernameInput } from '../../../../utils/account'
import type { TenantsApi } from '../../../../api/tenants/api'
import type { Status, Tenant } from '../../../../api/tenants/types'
export function TenantForm({
  api,
  tenant,
  onClose,
  onSaved,
}: {
  api: TenantsApi
  tenant?: Tenant
  onClose: () => void
  onSaved: (tenant: Tenant) => void
}) {
  const [name, setName] = useState(tenant?.name ?? '')
  const [hostname, setHostname] = useState(tenant?.hostname ?? '')
  const [status, setStatus] = useState<Status>(tenant?.status ?? 'active')
  const [registration, setRegistration] = useState(tenant?.registration_enabled ?? false)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [validation, setValidation] = useState('')
  const action = useAction()
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const error = !name.trim() ? '请输入企业名称' : !tenant ? passwordError(password) : ''
    setValidation(error)
    if (error) return
    void action.run(async (signal) => {
      if (tenant)
        return api.platformUpdateTenant(
          tenant.id,
          {
            name: name.trim(),
            hostname: hostname.trim().toLowerCase(),
            status,
            registration_enabled: registration,
          },
          signal,
        )
      const result = await api.createTenant(
        {
          name: name.trim(),
          hostname: hostname.trim().toLowerCase(),
          admin_username: username,
          admin_password: password,
        },
        signal,
      )
      return result.tenant
    }, onSaved)
  }
  return (
    <Modal
      title={tenant ? '编辑企业' : '开通企业空间'}
      description={
        tenant ? '管理企业基本信息与访问权限。' : '创建独立的协作空间，并设置第一位管理员。'
      }
      onClose={onClose}
      busy={action.busy}
    >
      <form className="stacked-form modal-form" onSubmit={submit}>
        <fieldset disabled={action.busy}>
          <Field label="企业名称">
            <input
              value={name}
              onChange={(event) => setName(event.target.value)}
              required
              placeholder="例如：云序科技"
            />
          </Field>
          <Field label="访问域名" hint="填写完整域名，不包含协议、端口或路径。">
            <input
              value={hostname}
              onChange={(event) => setHostname(event.target.value)}
              required
              autoCapitalize="none"
              spellCheck={false}
              placeholder="例如：team.example.com"
            />
          </Field>
          {tenant ? (
            <>
              <Field label="企业状态">
                <select
                  value={status}
                  onChange={(event) =>
                    setStatus(event.target.value === 'active' ? 'active' : 'disabled')
                  }
                >
                  <option value="active">正常 · 允许成员访问</option>
                  <option value="disabled">停用 · 禁止所有成员访问</option>
                </select>
              </Field>
              <label className="checkbox-field">
                <input
                  type="checkbox"
                  checked={registration}
                  onChange={(event) => setRegistration(event.target.checked)}
                />
                <span>
                  <strong>开放自主注册</strong>
                  <small>允许访问该企业域名的用户自行创建成员账号。</small>
                </span>
              </label>
              {status === 'disabled' && (
                <div className="notice warning">
                  <Icon name="alert" size={18} />
                  <span>保存后，企业成员将立即失去访问权限。</span>
                </div>
              )}
            </>
          ) : (
            <>
              <div className="form-divider">
                <span>首位企业管理员</span>
              </div>
              <Field label="管理员用户名" hint="3–64 位字母、数字、下划线、点或横线。">
                <input
                  value={username}
                  onChange={(event) => setUsername(event.target.value)}
                  {...usernameInput}
                  autoComplete="off"
                  placeholder="设置管理员登录名"
                />
              </Field>
              <Field label="管理员初始密码" hint="10–72 字节，请通过安全方式交给企业管理员。">
                <input
                  type="password"
                  {...newPasswordInput}
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  placeholder="设置初始密码"
                />
              </Field>
            </>
          )}
        </fieldset>
        <ErrorNotice message={validation || action.error} />
        <div className="modal-footer">
          <button
            className="button secondary"
            type="button"
            onClick={onClose}
            disabled={action.busy}
          >
            取消
          </button>
          <button className="button primary" type="submit" disabled={action.busy}>
            {action.busy ? '正在保存…' : tenant ? '保存企业信息' : '开通企业'}
          </button>
        </div>
      </form>
    </Modal>
  )
}
