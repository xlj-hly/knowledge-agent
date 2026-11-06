import type { Api } from '../../api/client'
import type { Identity } from '../../stores/session/types'
import { useIdentity } from '../../hooks/useIdentity'
import { ErrorNotice, Loading } from '../../components/ui'
import { AccountForms } from './components/AccountForms'
import { PlatformAccountSummary } from './components/PlatformAccountSummary'

/** 路由重新进入时读取账号，避免沿用取消等待前的写入结果与资料草稿。 */
export function AccountPage({ api, platform, onChanged, onSessionEnd }: {
  api: Api
  platform: boolean
  onChanged: (identity: Identity) => void
  onSessionEnd: (notice: string) => void
}) {
  const identity = useIdentity(api, platform, onChanged)
  if (identity.loading) return <Loading label="正在确认最新账号与企业信息…" />
  if (identity.error) return <ErrorNotice message={identity.error} retry={identity.refresh} />
  const fresh = identity.current
  if (!fresh) return null
  return (
    <AccountForms
      api={api}
      user={'user' in fresh ? fresh.user : null}
      summary={'admin' in fresh ? <PlatformAccountSummary admin={fresh.admin} /> : undefined}
      passwordTarget={'user' in fresh ? `tenant:${fresh.user.tenant_id}:${fresh.user.id}` : `platform:${fresh.admin.id}`}
      onUserUpdated={(next) => {
        if ('tenant' in fresh) identity.update({ user: next, tenant: fresh.tenant })
      }}
      changePassword={(body, signal) =>
        platform ? api.platformPassword(body, signal) : api.password(body, signal)
      }
      onPasswordChanged={() => onSessionEnd('密码已更新，请使用新密码重新登录。')}
    />
  )
}
