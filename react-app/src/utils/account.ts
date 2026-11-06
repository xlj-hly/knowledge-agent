export const usernameInput = {
  minLength: 3,
  maxLength: 64,
  pattern: '[A-Za-z0-9_.\\-]{3,64}',
  autoCapitalize: 'none',
  spellCheck: false,
  required: true,
} as const
export const newPasswordInput = {
  maxLength: 72,
  required: true,
  autoComplete: 'new-password',
} as const
// HTML maxlength counts UTF-16 units; 64 Unicode characters can occupy 128 units.
export const nicknameInput = { maxLength: 128 } as const
export function nicknameError(nickname: string): string {
  return Array.from(nickname).length > 64 ? '昵称最多 64 字符' : ''
}
export function passwordError(password: string): string {
  const bytes = new TextEncoder().encode(password).length
  return bytes < 10 || bytes > 72 ? '密码须为 10–72 字节（中文等字符占多个字节）' : ''
}
