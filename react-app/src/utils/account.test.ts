import { expect, test } from 'vitest'
import { nicknameError, passwordError } from './account'

test('新密码按 UTF-8 字节校验，中文密码可低于十个字符', () => {
  expect(passwordError('测试密码')).toBe('')
  expect(passwordError('短密码')).toMatch(/10–72/)
  expect(passwordError('密'.repeat(25))).toMatch(/10–72/)
})

test('昵称按 Unicode 字符计数，64 个 emoji 应能提交，65 个字符被拒绝', () => {
  expect(nicknameError('🙂'.repeat(64))).toBe('')
  expect(nicknameError('🙂'.repeat(65))).toMatch(/64/)
})
