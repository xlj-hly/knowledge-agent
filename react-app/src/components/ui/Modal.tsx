import { useEffect, useId, useRef } from 'react'
import type { ReactNode } from 'react'
import { Icon } from './Icon'
export function Modal({
  title,
  description,
  children,
  onClose,
  busy = false,
}: {
  title: string
  description: string
  children: ReactNode
  onClose: () => void
  busy?: boolean
}) {
  const dialog = useRef<HTMLDialogElement>(null)
  const titleId = useId()
  useEffect(() => {
    const node = dialog.current
    node?.showModal()
    return () => node?.close()
  }, [])
  return (
    <dialog
      className="modal"
      ref={dialog}
      aria-labelledby={titleId}
      onCancel={(event) => {
        event.preventDefault()
        if (!busy) onClose()
      }}
    >
      <div className="modal-heading">
        <div>
          <h2 id={titleId}>{title}</h2>
          <p>{description}</p>
        </div>
        <button
          className="icon-button"
          type="button"
          aria-label="关闭弹窗"
          disabled={busy}
          onClick={onClose}
        >
          <Icon name="x" />
        </button>
      </div>
      {children}
    </dialog>
  )
}
