import { useState, type FormEvent } from 'react'
import { ArrowRight, LockKeyhole } from 'lucide-react'
import { useLocation, useNavigate } from 'react-router'
import { APIError } from '@/api/client'
import { useApp } from '@/stores/app'
import { useAuth } from '@/stores/auth'

export default function LoginView() {
  const app = useApp()
  const auth = useAuth()
  const location = useLocation()
  const navigate = useNavigate()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const canSubmit = username.trim() !== '' && password !== '' && !auth.busy

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!canSubmit) return
    setError('')
    try {
      await auth.login(username, password)
      const requested = new URLSearchParams(location.search).get('next')
      const next = requested?.startsWith('/') && !requested.startsWith('//') ? requested : '/'
      await navigate(next, { replace: true })
    } catch (cause) {
      setError(cause instanceof APIError ? cause.message : '暂时无法登录，请稍后重试。')
    }
  }

  return (
    <main className="login-page">
      <section className="login-panel" aria-labelledby="login-title">
        <div className="brand-mark" aria-hidden="true">M</div>
        <p className="product-name">{app.name}</p>
        <h1 id="login-title">登录</h1>
        <p className="login-subtitle">使用管理员账号继续。</p>
        <form className="login-form" onSubmit={(event) => void submit(event)}>
          <label htmlFor="username">用户名</label>
          <input id="username" value={username} onChange={(event) => setUsername(event.target.value)} name="username" autoComplete="username" autoFocus required />
          <label htmlFor="password">密码</label>
          <div className="password-field">
            <LockKeyhole size={17} aria-hidden="true" />
            <input id="password" value={password} onChange={(event) => setPassword(event.target.value)} name="password" type="password" autoComplete="current-password" required />
          </div>
          {error && <p className="form-error" role="alert">{error}</p>}
          <button className="primary-button login-button" type="submit" disabled={!canSubmit}>
            <span>{auth.busy ? '正在登录...' : '登录'}</span>
            <ArrowRight size={17} aria-hidden="true" />
          </button>
        </form>
      </section>
    </main>
  )
}
