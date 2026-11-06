import { Link } from 'react-router'
import { Icon, PageHeader } from './ui'

export function AccessDenied({ home }: { home: string }) {
  return (
    <section className="panel">
      <PageHeader
        eyebrow="ACCESS DENIED"
        title="无权访问"
        description="此页面需要企业管理员权限。"
      />
      <Link to={home} className="button secondary"><Icon name="arrow" />返回工作台</Link>
    </section>
  )
}
