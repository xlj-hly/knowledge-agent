import { useRef, useState } from 'react'
import type { FormEvent } from 'react'
import type { AccountApi } from '../../../api/account/api'
import type { User } from '../../../api/account/types'
import { useAction } from '../../../hooks/async'
import { Avatar, Icon, Field, ErrorNotice, SuccessNotice } from '../../../components/ui'
import { nicknameError, nicknameInput } from '../../../utils/account'
export function Profile({
  api,
  user,
  onUpdated,
}: {
  api: AccountApi
  user: User
  onUpdated: (user: User) => void
}) {
  const [nickname, setNickname] = useState(user.nickname)
  const [avatar, setAvatar] = useState<File | null>(null)
  const avatarInput = useRef<HTMLInputElement>(null)
  const [success, setSuccess] = useState('')
  const [validation, setValidation] = useState('')
  const action = useAction()
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSuccess('')
    const error = nicknameError(nickname.trim())
    setValidation(error)
    if (error) return
    void action.run(
      async (signal) => {
        const avatarUrl = avatar ? await api.uploadAvatar(avatar, signal) : user.avatar_url
        // 上传完成后身份已变更时不能继续把旧头像写入资料。
        signal.throwIfAborted()
        return api.profile({ nickname: nickname.trim(), avatar_url: avatarUrl }, signal)
      },
      (next) => {
        setAvatar(null)
        if (avatarInput.current) avatarInput.current.value = ''
        onUpdated(next)
        setSuccess('个人资料已保存。')
      },
    )
  }
  return (
    <section className="panel account-panel">
      <div className="section-heading">
        <span className="section-icon">
          <Icon name="user" />
        </span>
        <div>
          <h2>个人资料</h2>
          <p>让团队成员更好地认识你。</p>
        </div>
      </div>
      <div className="profile-summary">
        <Avatar
          key={user.avatar_url}
          name={user.nickname || user.username}
          url={user.avatar_url}
          large
        />
        <div>
          <h3>{user.nickname || user.username}</h3>
          <span className="muted">@{user.username}</span>
        </div>
      </div>
      <form className="stacked-form" onSubmit={submit}>
        <Field label="用户名" hint="用户名用于登录，创建后不可修改。">
          <input value={user.username} disabled />
        </Field>
        <Field label="昵称">
          <input
            value={nickname}
            {...nicknameInput}
            autoComplete="nickname"
            disabled={action.busy}
            onChange={(event) => setNickname(event.target.value)}
          />
        </Field>
        <Field label="选择头像" hint="支持 JPG、PNG 图片，最大 2MB；保存资料后生效。">
          <input
            ref={avatarInput}
            type="file"
            accept="image/jpeg,image/png"
            disabled={action.busy}
            onChange={(event) => {
              setSuccess('')
              const file = event.target.files?.[0] ?? null
              const error = file && (!['image/jpeg', 'image/png'].includes(file.type) || file.size > 2 * 1024 * 1024)
                ? '请选择不超过 2MB 的 JPG 或 PNG 图片'
                : ''
              setValidation(error)
              setAvatar(error ? null : file)
              if (error) event.target.value = ''
            }}
          />
        </Field>
        <ErrorNotice message={validation || action.error} />
        <SuccessNotice message={success} />
        <div className="form-footer">
          <button className="button primary" type="submit" disabled={action.busy}>
            {action.busy ? '正在保存…' : '保存个人资料'}
          </button>
        </div>
      </form>
    </section>
  )
}
