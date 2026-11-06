function byteLength(value) {
  return Array.from(value).reduce((length, char) => {
    const code = char.codePointAt(0)
    return length + (code <= 0x7f ? 1 : code <= 0x7ff ? 2 : code <= 0xffff ? 3 : 4)
  }, 0)
}

function passwordError(password) {
  const length = byteLength(password || '')
  return length < 10 || length > 72 ? '密码需为 10–72 字节，中文等字符会占用多个字节。' : ''
}

function nicknameError(nickname) {
  return Array.from(nickname || '').length > 64 ? '昵称最多 64 个字符。' : ''
}

export function validateCredentials({ username, password }) {
  if (!/^[a-z0-9_.-]{3,64}$/i.test((username || '').trim())) return '用户名需为 3–64 个英文字母、数字、下划线、点或横线。'
  return passwordError(password)
}

export function validateProfile({ nickname, avatar_url }) {
  const nicknameProblem = nicknameError(nickname)
  if (nicknameProblem) return nicknameProblem
  const url = (avatar_url || '').trim()
  if (url && ((!/^https?:\/\/[^/\s?#]+(?:[/?#][^\s]*)?$/i.test(url) && !/^\/api\/avatars\/[1-9]\d*\/[1-9]\d*\/[a-zA-Z0-9_-]+\.(?:png|jpg)$/.test(url)) || byteLength(url) > 2048)) {
    return '头像地址无效，请重新选择头像（http(s) 或已上传的图片）。'
  }
  return ''
}

export function validatePasswordChange({ current_password, new_password, confirmation }, wechat = false) {
  if (!wechat && !current_password) return '请填写当前密码。'
  return passwordError(new_password) || (new_password !== confirmation ? '两次填写的新密码不一致。' : '')
}
