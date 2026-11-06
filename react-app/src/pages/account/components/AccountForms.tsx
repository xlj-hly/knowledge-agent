import type { ReactNode } from 'react'
import type { AccountApi } from '../../../api/account/api'
import type { User, PasswordChange } from '../../../api/account/types'
import { PageHeader } from '../../../components/ui'
import { Profile } from './Profile'
import { PasswordForm } from './PasswordForm'
import '../account.css'

/** 调用方组合身份摘要，并在密码操作所属身份变化时同步更换 passwordTarget。 */
export function AccountForms({
  api,
  user,
  summary,
  passwordTarget,
  onUserUpdated,
  changePassword,
  onPasswordChanged,
}: {
  api: AccountApi
  user: User | null
  summary?: ReactNode
  passwordTarget: string
  onUserUpdated: (user: User) => void
  changePassword: (body: PasswordChange, signal: AbortSignal) => Promise<void>
  onPasswordChanged: () => void
}) {
  return (
    <>
      <PageHeader
        eyebrow="YOUR ACCOUNT"
        title="个人账号"
        description="管理你的个人资料与登录安全。"
      />
      <div className="account-grid">
        {user && (
          <Profile
            key={`${user.tenant_id}:${user.id}`}
            api={api}
            user={user}
            onUpdated={(next) => onUserUpdated(next)}
          />
        )}
        {summary}
        <PasswordForm
          key={passwordTarget}
          changePassword={(body, signal) => changePassword(body, signal)}
          onChanged={onPasswordChanged}
        />
      </div>
    </>
  )
}
