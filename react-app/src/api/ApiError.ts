export class ApiError extends Error {
  status: number
  code: string
  constructor(message: string, code: string, status = 0) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}
