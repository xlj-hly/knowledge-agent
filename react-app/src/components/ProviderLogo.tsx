import { Icon } from './ui/Icon'
import './ProviderLogo.css'

export function ProviderLogo({ logo, size = 32 }: { logo?: string; size?: number }) {
  return <span className="provider-logo" style={{ width: size, height: size }} aria-hidden="true">
    {logo ? <img src={logo} alt="" width={size} height={size} /> : <Icon name="spark" size={size} />}
  </span>
}
