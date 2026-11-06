const timeout = 10000

function loginError(code, message) {
  return Object.assign(new Error(message), { code })
}

/** 页面拥有适配器生命周期；取消只丢弃本次 code，不能中断微信平台内部登录。 */
export function createWechatLogin(uni) {
  let cancelPending = () => {}

  function cancel() {
    cancelPending()
  }

  function getCode() {
    cancel()
    return new Promise((resolve, reject) => {
      let settled = false
      let timer
      const finish = (error, code) => {
        if (settled) return
        settled = true
        clearTimeout(timer)
        cancelPending = () => {}
        if (error) reject(error)
        else resolve(code)
      }
      cancelPending = () => finish(loginError('CANCELLED', '微信登录已取消'))
      // 平台回调可能不返回，使用独立期限确保界面可重试。
      timer = setTimeout(() => finish(loginError('WECHAT_LOGIN_TIMEOUT', '微信登录超时，请重试。')), timeout)
      try {
        uni.login({
          provider: 'weixin',
          timeout,
          success(result) {
            if (typeof result?.code !== 'string' || !/^[A-Za-z0-9_-]{1,256}$/.test(result.code)) {
              finish(loginError('WECHAT_INVALID_CODE', '未取得有效的微信登录凭证，请重试。'))
              return
            }
            finish(null, result.code)
          },
          fail() {
            finish(loginError('WECHAT_LOGIN_FAILED', '无法完成微信登录，请重试。'))
          },
        })
      } catch {
        finish(loginError('WECHAT_LOGIN_FAILED', '微信登录暂时不可用，请重试。'))
      }
    })
  }

  return { getCode, cancel }
}
