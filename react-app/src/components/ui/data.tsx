import { useState } from 'react'
import { Icon } from './Icon'
export function Pagination({
  page,
  total,
  pageSize,
  onChange,
}: {
  page: number
  total: number
  pageSize: number
  onChange: (page: number) => void
}) {
  const pages = Math.max(1, Math.ceil(total / pageSize))
  return (
    <div className="pagination">
      <span>
        共 <strong>{total}</strong> 条记录 · 每页 {pageSize} 条
      </span>
      <div>
        <button
          className="button secondary compact"
          disabled={page <= 1}
          onClick={() => onChange(page - 1)}
        >
          上一页
        </button>
        <span>
          {page} / {pages}
        </span>
        <button
          className="button secondary compact"
          disabled={page >= pages}
          onClick={() => onChange(page + 1)}
        >
          下一页
        </button>
      </div>
    </div>
  )
}
export function SearchForm({
  onSearch,
  placeholder,
}: {
  onSearch: (value: string) => void
  placeholder: string
}) {
  const [value, setValue] = useState('')
  return (
    <form
      className="search-form"
      role="search"
      onSubmit={(event) => {
        event.preventDefault()
        onSearch(value.trim())
      }}
    >
      <div>
        <Icon name="search" size={18} />
        <input
          aria-label={placeholder}
          placeholder={placeholder}
          value={value}
          onChange={(event) => setValue(event.target.value)}
        />
      </div>
      <button className="button secondary compact" type="submit">
        搜索
      </button>
    </form>
  )
}
