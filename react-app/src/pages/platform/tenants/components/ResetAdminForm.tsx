import { useState } from 'react'
import type { FormEvent } from 'react'
import { useAction } from '../../../../hooks/async'
import { ErrorNotice, Field, Icon, Modal } from '../../../../components/ui'
import { newPasswordInput, passwordError, usernameInput } from '../../../../utils/account'
import type { TenantsApi } from '../../../../api/tenants/api'
import type { Tenant } from '../../../../api/tenants/types'
export function ResetAdminForm({
  api,
  tenant,
  onClose,
  onSaved,
}: {
  api: TenantsApi
  tenant: Tenant
  onClose: () => void
  onSaved: () => void
}) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [validation, setValidation] = useState('')
  const action = useAction()
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const error = passwordError(password)
    setValidation(error)
    if (error) return
    void action.run(
      (signal) => api.resetAdminPassword(tenant.id, { username, new_password: password }, signal),
      onSaved,
    )
  }
  return (
    <Modal
      title="重置管理员密码"
      description={`为「${tenant.name}」中已有的管理员设置新密码。`}
      onClose={onClose}
      busy={action.busy}
    >
      <form className="stacked-form modal-form" onSubmit={submit}>
        <fieldset disabled={action.busy}>
          <Field label="管理员用户名">
            <input
              {...usernameInput}
              autoComplete="off"
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              placeholder="输入现有企业管理员用户名"
            />
          </Field>
          <Field label="新密码" hint="10–72 字节，重置后该管理员的所有会话立即失效。">
            <input
              type="password"
              {...newPasswordInput}
              value={password}
              onChange={(event) => setPassword(event.target.value)}
            />
          </Field>
        </fieldset>
        <ErrorNotice message={validation || action.error} />
        <div className="notice warning">
          <Icon name="lock" size={18} />
          <span>请通过安全方式告知管理员新密码。</span>
        </div>
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
            {action.busy ? '正在重置…' : '确认重置密码'}
          </button>
        </div>
      </form>
    </Modal>
  )
}
