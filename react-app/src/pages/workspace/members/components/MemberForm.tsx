import { useState } from 'react'
import type { FormEvent } from 'react'
import { useAction } from '../../../../hooks/async'
import { Avatar, ErrorNotice, Field, Icon, Modal } from '../../../../components/ui'
import {
  newPasswordInput,
  nicknameError,
  nicknameInput,
  passwordError,
  usernameInput,
} from '../../../../utils/account'
import type { MembersApi } from '../../../../api/members'
import type { Role, User, UserStatus } from '../../../../api/account/types'
export function MemberForm({
  api,
  user,
  onClose,
  onSaved,
}: {
  api: MembersApi
  user?: User
  onClose: () => void
  onSaved: (user: User) => void
}) {
  const [username, setUsername] = useState('')
  const [nickname, setNickname] = useState('')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState<Role>(user?.role ?? 'user')
  const [status, setStatus] = useState<UserStatus>(user?.status ?? 'active')
  const [validation, setValidation] = useState('')
  const action = useAction()
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const error = user ? '' : passwordError(password) || nicknameError(nickname.trim())
    setValidation(error)
    if (error) return
    void action.run(
      (signal) =>
        user
          ? api.updateUser(user.id, { role, status }, signal)
          : api.createUser({ username, nickname: nickname.trim(), password, role }, signal),
      onSaved,
    )
  }
  return (
    <Modal
      title={user ? '编辑成员权限' : '添加团队成员'}
      description={
        user
          ? `调整「${user.nickname || user.username}」的角色与访问状态。`
          : '创建新账号，让更多伙伴加入企业空间。'
      }
      onClose={onClose}
      busy={action.busy}
    >
      <form className="stacked-form modal-form" onSubmit={submit}>
        <fieldset disabled={action.busy}>
          {user ? (
            <div className="editing-member">
              <Avatar name={user.nickname || user.username} url={user.avatar_url} />
              <div>
                <strong>{user.nickname || user.username}</strong>
                <span>@{user.username}</span>
              </div>
            </div>
          ) : (
            <>
              <Field label="用户名" hint="3–64 位字母、数字、下划线、点或横线，创建后不可修改。">
                <input
                  {...usernameInput}
                  autoComplete="off"
                  value={username}
                  onChange={(event) => setUsername(event.target.value)}
                  placeholder="设置成员登录名"
                />
              </Field>
              <Field label="昵称（可选）">
                <input
                  value={nickname}
                  {...nicknameInput}
                  onChange={(event) => setNickname(event.target.value)}
                  placeholder="团队如何称呼这位成员"
                />
              </Field>
              <Field label="初始密码" hint="10–72 字节，请通过安全方式告知成员。">
                <input
                  type="password"
                  {...newPasswordInput}
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                />
              </Field>
            </>
          )}
          <Field label="成员角色">
            <select
              value={role}
              onChange={(event) => setRole(event.target.value === 'admin' ? 'admin' : 'user')}
            >
              <option value="user">普通成员 · 管理个人账号</option>
              <option value="admin">企业管理员 · 管理成员与企业</option>
            </select>
          </Field>
          {user && (
            <Field label="账号状态">
              <select
                value={status}
                onChange={(event) =>
                  setStatus(event.target.value === 'active' ? 'active' : 'disabled')
                }
              >
                <option value="active">正常 · 允许登录</option>
                <option value="disabled">停用 · 禁止登录</option>
              </select>
            </Field>
          )}
          {status === 'disabled' && (
            <div className="notice warning">
              <Icon name="alert" size={18} />
              <span>停用后，该成员将立即失去访问权限。</span>
            </div>
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
            {action.busy ? '正在保存…' : user ? '保存成员权限' : '添加成员'}
          </button>
        </div>
      </form>
    </Modal>
  )
}
