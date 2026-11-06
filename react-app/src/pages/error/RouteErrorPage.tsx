import { Link } from 'react-router'
import { Icon, PageHeader } from '../../components/ui'
import { AccessDenied } from '../../components/AccessDenied'

export function RouteErrorPage({ forbidden = false, home }: { forbidden?: boolean; home: string }) {
  if (forbidden) return <AccessDenied home={home} />
  return (
    <section className="panel">
      <PageHeader eyebrow="PAGE NOT FOUND"
        title="页面不存在"
        description="请检查地址，或返回工作台继续。" />
      <Link to={home} className="button secondary"><Icon name="arrow" />返回工作台</Link>
    </section>
  )
}
