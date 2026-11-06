import { useState } from 'react'
import type { FormEvent } from 'react'
import type { PasswordChange } from '../../../api/account/types'
import { useAction } from '../../../hooks/async'
import { Field, Icon, ErrorNotice } from '../../../components/ui'
import { newPasswordInput, passwordError } from '../../../utils/account'
export function PasswordForm({
  changePassword,
  onChanged,
}: {
  changePassword: (value: PasswordChange, signal: AbortSignal) => Promise<void>
  onChanged: () => void
}) {
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [validation, setValidation] = useState('')
  const action = useAction()
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const error =
      passwordError(newPassword) || (newPassword !== confirmation ? '两次输入的新密码不一致' : '')
    setValidation(error)
    if (error) return
    void action.run(
      (signal) =>
        changePassword({ current_password: currentPassword, new_password: newPassword }, signal),
      onChanged,
    )
  }
  return (
    <section className="panel account-panel">
      <div className="section-heading">
        <span className="section-icon">
          <Icon name="lock" />
        </span>
        <div>
          <h2>修改登录密码</h2>
          <p>更新密码后，所有设备需要重新登录。</p>
        </div>
      </div>
      <form className="stacked-form" onSubmit={submit}>
        <Field label="当前密码">
          <input
            type="password"
            autoComplete="current-password"
            required
            value={currentPassword}
            onChange={(event) => setCurrentPassword(event.target.value)}
          />
        </Field>
        <Field label="新密码" hint="10–72 字节，建议组合使用字母、数字和符号。">
          <input
            type="password"
            {...newPasswordInput}
            value={newPassword}
            onChange={(event) => setNewPassword(event.target.value)}
          />
        </Field>
        <Field label="确认新密码">
          <input
            type="password"
            {...newPasswordInput}
            value={confirmation}
            onChange={(event) => setConfirmation(event.target.value)}
          />
        </Field>
        <ErrorNotice message={validation || action.error} />
        <div className="form-footer">
          <button className="button primary" type="submit" disabled={action.busy}>
            {action.busy ? '正在更新…' : '更新密码并重新登录'}
          </button>
        </div>
      </form>
    </section>
  )
}
