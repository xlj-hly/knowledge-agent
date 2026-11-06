import { useState } from 'react'
import type { FormEvent } from 'react'
import type { AuthApi } from '../../api/auth/api'
import { Link } from 'react-router'
import { useAction } from '../../hooks/async'
import type { Tenant } from '../../api/tenants/types'
import { Brand, Icon, Field, ErrorNotice, SuccessNotice } from '../../components/ui'
import { nicknameError, nicknameInput, passwordError, usernameInput } from '../../utils/account'
import './login.css'
const currentYear = new Date().getFullYear()
const hostname = window.location.hostname
export function LoginPage({
  api,
  platform,
  tenant,
  homePath,
  notice,
  sessionError = '',
  onLogin,
}: {
  api: AuthApi
  platform: boolean
  tenant?: Tenant
  homePath: string
  notice: string
  sessionError?: string
  onLogin: (token: string) => void
}) {
  const [register, setRegister] = useState(false)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [nickname, setNickname] = useState('')
  const [validation, setValidation] = useState('')
  const action = useAction()
  const registrationOpen = !platform && tenant?.registration_enabled === true
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const error = register ? passwordError(password) || nicknameError(nickname) : ''
    setValidation(error)
    if (error) return
    void action.run(async (signal) => {
      const session = await (platform
        ? api.platformLogin({ username, password }, signal)
        : register
          ? api.register({ username, password, nickname }, signal)
          : api.login({ username, password }, signal))
      return session.token
    }, onLogin)
  }
  const tenantUnavailable =
    !platform && (!tenant || tenant.status !== 'active')
  return (
    <div className="auth-page">
      <section className="auth-story">
        <Link className="brand-link" to={homePath}>
          <Brand platform={platform} />
        </Link>
        <div className="auth-story-main">
          <span className="auth-tag">
            <span />
            {platform ? '有序管理，连接每一家企业' : '属于你的企业协作空间'}
          </span>
          <h1>
            让每一次协作，
            <br />
            都有清晰的<span>秩序。</span>
          </h1>
          <p>
            从团队与账号开始，
            <br />
            连接每一位成员，专注每一步成长。
          </p>
          <div className="auth-art" aria-hidden="true">
            <div className="art-orbit orbit-one" />
            <div className="art-orbit orbit-two" />
            <div className="art-window">
              <div className="art-dots">
                <i />
                <i />
                <i />
              </div>
              <div className="art-sidebar">
                <b />
                <b />
                <b />
              </div>
              <div className="art-content">
                <span />
                <span />
                <div>
                  <i />
                  <i />
                  <i />
                </div>
                <span />
              </div>
            </div>
            <div className="art-float">
              <Icon name="shield" size={22} />
              <div>
                <strong>安全、有序、独立</strong>
                <small>为每一个团队而设计</small>
              </div>
              <Icon name="check" size={16} />
            </div>
          </div>
        </div>
        <span className="auth-story-footer">
          YUNXU WORKSPACE <span>© {currentYear}</span>
        </span>
      </section>
      <section className="auth-form-side">
        <div className="auth-top-note">
          {platform ? '平台管理入口' : hostname}
          <Icon name={platform ? 'shield' : 'globe'} size={15} />
        </div>
        <div className="auth-card">
          <div className="auth-heading-icon">
            <Icon name={platform ? 'building' : 'users'} size={26} />
          </div>
          <span className="eyebrow">
            {platform ? 'PLATFORM CONSOLE' : 'WELCOME TO YOUR WORKSPACE'}
          </span>
          <h2>{platform ? '欢迎回到管理控制台' : register ? '加入你的企业' : '欢迎回来'}</h2>
          <p>
            {platform
              ? '使用平台管理员账号登录，管理企业与访问权限。'
              : tenant
                ? `${tenant.name} · ${register ? '创建账号，开始有序协作' : '登录你的企业协作空间'}`
                : '正在连接你的企业空间'}
          </p>
          <SuccessNotice message={notice} />
          <ErrorNotice message={sessionError} />
          {tenant?.status === 'disabled' && (
            <ErrorNotice message="当前企业已停用，请联系平台管理员。" />
          )}
          {!tenantUnavailable && (
            <form className="stacked-form auth-form" onSubmit={submit}>
              <Field label="用户名">
                <input
                  name="username"
                  autoComplete="username"
                  placeholder="请输入用户名"
                  {...usernameInput}
                  value={username}
                  onChange={(event) => setUsername(event.target.value)}
                />
              </Field>
              {register && (
                <Field label="昵称（可选）">
                  <input
                    name="nickname"
                    autoComplete="nickname"
                    {...nicknameInput}
                    placeholder="团队成员如何称呼你"
                    value={nickname}
                    onChange={(event) => setNickname(event.target.value)}
                  />
                </Field>
              )}
              <Field
                label="密码"
                hint={register ? '10–72 字节，建议组合使用字母、数字和符号' : undefined}
              >
                <input
                  name="password"
                  type="password"
                  autoComplete={register ? 'new-password' : 'current-password'}
                  placeholder={register ? '设置登录密码' : '请输入密码'}
                  maxLength={72}
                  required
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                />
              </Field>
              <ErrorNotice message={validation || action.error} />
              <button className="button primary auth-submit" type="submit" disabled={action.busy}>
                {action.busy ? '正在处理…' : register ? '创建账号并加入' : '登录工作空间'}
                {!action.busy && <Icon name="arrow" size={17} />}
              </button>
            </form>
          )}
          {registrationOpen && (
            <p className="auth-switch">
              {register ? '已有账号？' : '还没有账号？'}
              <button
                type="button"
                onClick={() => {
                  setRegister(!register)
                  setPassword('')
                  setValidation('')
                }}
                disabled={action.busy}
              >
                {register ? '返回登录' : '注册加入企业'}
                <Icon name="arrow" size={14} />
              </button>
            </p>
          )}
          {!platform && tenant && !registrationOpen && (
            <p className="auth-help">企业暂未开放自主注册，请联系管理员获取账号。</p>
          )}
          <div className="auth-security">
            <Icon name="shield" size={14} />
            企业空间独立隔离，账号安全始终在线
          </div>
        </div>
        <p className="auth-footer">简洁的体验 · 可靠的连接 · 有序的协作</p>
      </section>
    </div>
  )
}
