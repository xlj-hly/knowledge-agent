import { Icon } from '../components/ui'
import { NavLink } from 'react-router'
export interface NavItemProps {
  icon: Parameters<typeof Icon>[0]['name']
  label: string
  to: string
  end?: boolean
  group?: 'ai' | 'management'
}

export function NavItem({
  icon,
  label,
  to,
  end = false,
}: NavItemProps) {
  return (
    <NavLink
      className={({ isActive }) => `nav-item${isActive ? ' active' : ''}`}
      to={to}
      end={end}
      aria-label={label}
      title={label}
    >
      {({ isActive }) => (
        <>
          <Icon name={icon} size={19} />
          {label}
          {isActive && <span />}
        </>
      )}
    </NavLink>
  )
}
