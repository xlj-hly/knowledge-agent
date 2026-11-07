import { ApiError } from './ApiError'
export function invalid(): never {
  throw new ApiError('服务返回的数据格式异常，请稍后重试', 'invalid_response')
}
export function record(value: unknown): Record<string, unknown> {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return invalid()
  return value as Record<string, unknown>
}
export function string(value: unknown): string {
  return typeof value === 'string' ? value : invalid()
}
export function id(value: unknown): string {
  const result = string(value)
  return /^[1-9]\d*$/.test(result) ? result : invalid()
}
export function bool(value: unknown): boolean {
  return typeof value === 'boolean' ? value : invalid()
}
/** 服务商图形由后端内置目录提供，避免配置响应引入任意远程图片请求。 */
export function providerLogo(value: unknown): string {
  const logo = string(value)
  return logo.length <= 256 * 1024 && /^data:image\/(?:svg\+xml|png|jpeg);base64,[A-Za-z0-9+/]+={0,2}$/.test(logo) ? logo : invalid()
}
export function integer(value: unknown, min = 0): number {
  return typeof value === 'number' && Number.isSafeInteger(value) && value >= min
    ? value
    : invalid()
}
export function decodePage<T>(decode: (value: unknown) => T) {
  return (value: unknown): Page<T> => {
    const row = record(value)
    if (!Array.isArray(row.items)) return invalid()
    return {
      items: row.items.map(decode),
      total: integer(row.total),
      page: integer(row.page, 1),
      page_size: integer(row.page_size, 1),
    }
  }
}
export function decodeOk(value: unknown): void {
  if (record(value).ok !== true) invalid()
}

export interface Page<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}
