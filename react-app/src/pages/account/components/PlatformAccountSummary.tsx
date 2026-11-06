import type { PlatformAdmin } from '../../../api/auth/types'
import { Avatar, Icon } from '../../../components/ui'
import { formatDate } from '../../../utils/date'
import '../platform-account.css'

export function PlatformAccountSummary({ admin }: { admin: PlatformAdmin }) {
  return (
    <section className="panel platform-account-summary">
      <div className="section-heading">
        <span className="section-icon">
          <Icon name="user" />
        </span>
        <div>
          <h2>平台管理员</h2>
          <p>独立的平台管理身份</p>
        </div>
      </div>
      <div className="platform-account-profile">
        <Avatar name={admin.username} large />
        <div>
          <h3>{admin.username}</h3>
          <span className="badge purple">平台管理员</span>
        </div>
      </div>
      <dl className="details">
        <div>
          <dt>账号创建于</dt>
          <dd>{formatDate(admin.created_at)}</dd>
        </div>
      </dl>
    </section>
  )
}
