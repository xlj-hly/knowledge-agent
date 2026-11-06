export type Role = 'admin' | 'user'
export type UserStatus = 'active' | 'disabled'

export interface Credentials {
  username: string
  password: string
}

export interface User {
  id: string
  tenant_id: string
  username: string
  nickname: string
  avatar_url: string
  role: Role
  status: UserStatus
  created_at: string
  updated_at: string
}

export interface PasswordChange {
  current_password: string
  new_password: string
}
