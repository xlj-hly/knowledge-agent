import { test, expect } from 'vitest'
import { validateCredentials, validateProfile, validatePasswordChange } from './validation.mjs'

test('credential checks enforce ASCII usernames and password byte length without trimming passwords', () => {
  expect(validateCredentials({ username: 'alice_1', password: '密'.repeat(24) })).toBe('')
  expect(validateCredentials({ username: '中文名', password: 'abcdefghij' })).toMatch(/用户名/)
  expect(validateCredentials({ username: 'alice', password: '密'.repeat(25) })).toMatch(/72/)
  expect(validateCredentials({ username: 'alice', password: 'short' })).toMatch(/10/)
  expect(validateCredentials({ username: 'alice', password: 'abcdefghij', nickname: '名'.repeat(65) }, true)).toMatch(/昵称/)
})

test('profile and password changes reject unsafe avatars and mismatched confirmations', () => {
  expect(validateProfile({ nickname: '小林', avatar_url: 'https://cdn.test/avatar.png' })).toBe('')
  expect(validateProfile({ nickname: '', avatar_url: 'javascript:alert(1)' })).toMatch(/http/)
  expect(validatePasswordChange({ current_password: 'oldpassword1', new_password: 'newpassword1', confirmation: 'newpassword1' })).toBe('')
  expect(validatePasswordChange({ current_password: 'oldpassword1', new_password: 'newpassword1', confirmation: 'different' })).toMatch(/不一致/)
})
