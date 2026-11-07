import { ErrorNotice } from './primitives'
import { Modal } from './Modal'
import { useAction } from '../../hooks/async'

export function ConfirmDelete({ title, description, onDelete, onClose, onDeleted }: {
  title: string
  description: string
  onDelete: (signal: AbortSignal) => Promise<void>
  onClose: () => void
  onDeleted: () => void
}) {
  const action = useAction()
  return <Modal title={title} description={description} onClose={onClose} busy={action.busy}>
    <form className="modal-form" onSubmit={event => { event.preventDefault(); void action.run(onDelete, onDeleted) }}>
      <ErrorNotice message={action.error} />
      <div className="modal-footer">
        <button type="button" className="button secondary" disabled={action.busy} onClick={onClose}>取消</button>
        <button className="button primary" disabled={action.busy}>{action.busy ? '正在删除…' : '确认删除'}</button>
      </div>
    </form>
  </Modal>
}
