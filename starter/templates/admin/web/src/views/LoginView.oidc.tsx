import { ArrowRight, KeyRound } from 'lucide-react'
import { useApp } from '@/stores/app'

export default function LoginView() {
  const app = useApp()

  return (
    <main className="login-page">
      <section className="login-panel" aria-labelledby="login-title">
        <div className="brand-mark" aria-hidden="true">M</div>
        <p className="product-name">{app.name}</p>
        <h1 id="login-title">登录</h1>
        <p className="login-subtitle">使用组织身份账号安全登录。</p>
        <a className="primary-button login-button" href="/api/auth/oidc/login">
          <KeyRound size={17} aria-hidden="true" />
          <span>使用企业账号登录</span>
          <ArrowRight size={17} aria-hidden="true" />
        </a>
      </section>
    </main>
  )
}
