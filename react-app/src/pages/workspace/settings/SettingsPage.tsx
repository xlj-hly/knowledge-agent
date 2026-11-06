import type { Api } from '../../../api/client'
import type { Identity } from '../../../stores/session/types'
import { useIdentity } from '../../../hooks/useIdentity'
import { ErrorNotice, Loading } from '../../../components/ui'
import { SettingsForm } from './components/SettingsForm'
import { AccessDenied } from '../../../components/AccessDenied'

export function SettingsPage({ api, onChanged }: { api: Api; onChanged: (identity: Identity) => void }) {
  const identity = useIdentity(api, false, onChanged)
  if (identity.loading) return <Loading label="正在确认最新账号与企业信息…" />
  if (identity.error) return <ErrorNotice message={identity.error} retry={identity.refresh} />
  const fresh = identity.current
  if (!fresh) return null
  if (!('user' in fresh) || fresh.user.role !== 'admin') return <AccessDenied home="/" />
  return (
    <SettingsForm key={fresh.tenant.id} api={api} tenant={fresh.tenant}
      onUpdated={next => identity.update({ user: fresh.user, tenant: next })} />
  )
}
