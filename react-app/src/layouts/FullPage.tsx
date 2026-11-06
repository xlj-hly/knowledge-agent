import type { ReactNode } from 'react'
import './layout.css'
export function FullPage({ children }: { children: ReactNode }) {
  return (
    <div className="full-page">
      <div className="full-page-card">{children}</div>
    </div>
  )
}
