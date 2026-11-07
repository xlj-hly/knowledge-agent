import { useLayoutEffect, useRef, useState } from 'react'

/** 阅读旧消息时停止跟随；换会话或主动发送后重新跟随底部。 */
export function useChatScroll(scope: string | undefined, revision: unknown) {
  const viewport = useRef<HTMLDivElement>(null)
  const following = useRef(true)
  const [position, setPosition] = useState({ scope, away: false })
  const away = position.scope === scope && position.away
  const toBottom = () => {
    following.current = true; setPosition({ scope, away: false })
    const node = viewport.current
    if (node) node.scrollTop = node.scrollHeight
  }
  useLayoutEffect(() => { following.current = true }, [scope])
  useLayoutEffect(() => { if (following.current && viewport.current) viewport.current.scrollTop = viewport.current.scrollHeight }, [scope, revision])
  const onScroll = () => {
    const node = viewport.current
    if (!node) return
    following.current = node.scrollHeight - node.scrollTop - node.clientHeight < 96
    setPosition({ scope, away: !following.current })
  }
  return { viewport, away, onScroll, toBottom }
}
