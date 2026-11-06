import { useState } from 'react'
import type { FormEvent } from 'react'
import { useAction } from '../../../../hooks/async'
import { ErrorNotice, Field, Icon, Modal } from '../../../../components/ui'
import { newPasswordInput, passwordError } from '../../../../utils/account'
import type { MembersApi } from '../../../../api/members'
import type { User } from '../../../../api/account/types'
export function ResetUserPassword({
  api,
  user,
  onClose,
  onSaved,
}: {
  api: MembersApi
  user: User
  onClose: () => void
  onSaved: () => void
}) {
  const [password, setPassword] = useState('')
  const [validation, setValidation] = useState('')
  const action = useAction()
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const error = passwordError(password)
    setValidation(error)
    if (error) return
    void action.run((signal) => api.resetUserPassword(user.id, password, signal), onSaved)
  }
  return (
    <Modal
      title="重置成员密码"
      description={`为「${user.nickname || user.username}」设置新的登录密码。`}
      onClose={onClose}
      busy={action.busy}
    >
      <form className="stacked-form modal-form" onSubmit={submit}>
        <Field label="新密码" hint="10–72 字节，重置后该成员的所有会话立即失效。">
          <input
            type="password"
            {...newPasswordInput}
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            disabled={action.busy}
          />
        </Field>
        <ErrorNotice message={validation || action.error} />
        <div className="notice warning">
          <Icon name="lock" size={18} />
          <span>请通过安全方式将新密码告知该成员。</span>
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
