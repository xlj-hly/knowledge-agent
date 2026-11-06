import { useState } from 'react'
import type { ReactNode } from 'react'
import { Icon } from './Icon'
export function Brand({ platform = false }: { platform?: boolean }) {
  return (
    <div className="brand">
      <span className="brand-mark">
        <Icon name="grid" size={24} />
      </span>
      <span>
        <strong>云序</strong>
        <small>{platform ? 'PLATFORM CONSOLE' : 'WORKSPACE'}</small>
      </span>
    </div>
  )
}
export function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: ReactNode
}) {
  return (
    <label className="field">
      <span>{label}</span>
      {children}
      {hint && <small>{hint}</small>}
    </label>
  )
}
export function ErrorNotice({ message, retry }: { message: string; retry?: () => void }) {
  if (!message) return null
  return (
    <div className="notice error" role="alert">
      <Icon name="alert" size={18} />
      <span>{message}</span>
      {retry && (
        <button type="button" onClick={retry}>
          重新加载
        </button>
      )}
    </div>
  )
}
export function SuccessNotice({ message }: { message: string }) {
  return message ? (
    <div className="notice success" role="status">
      <Icon name="check" size={18} />
      <span>{message}</span>
    </div>
  ) : null
}
export function Loading({ label = '正在加载…' }: { label?: string }) {
  return (
    <div className="loading" role="status">
      <span className="spinner" />
      {label}
    </div>
  )
}
export function StatusBadge({ status }: { status: 'active' | 'disabled' }) {
  return (
    <span className={`badge ${status === 'active' ? 'green' : 'gray'}`}>
      <i />
      {status === 'active' ? '正常' : '已停用'}
    </span>
  )
}
export function Avatar({
  name,
  url,
  large = false,
}: {
  name: string
  url?: string
  large?: boolean
}) {
  const [failedUrl, setFailedUrl] = useState<string | null>(null)
  return (
    <span className={`avatar${large ? ' large' : ''}`}>
      {url && failedUrl !== url ? (
        <img
          src={url}
          alt="个人头像"
          referrerPolicy="no-referrer"
          onError={() => setFailedUrl(url)}
        />
      ) : (
        name.slice(0, 1).toUpperCase()
      )}
    </span>
  )
}
export function EmptyState({ title, description }: { title: string; description: string }) {
  return (
    <div className="empty-state">
      <span>
        <Icon name="search" size={26} />
      </span>
      <h3>{title}</h3>
      <p>{description}</p>
    </div>
  )
}
export function PageHeader({
  eyebrow,
  title,
  description,
  action,
}: {
  eyebrow: string
  title: string
  description: string
  action?: ReactNode
}) {
  return (
    <div className="page-header">
      <div>
        <span className="eyebrow">{eyebrow}</span>
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
      {action}
    </div>
  )
}
