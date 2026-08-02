import { useEffect } from 'react'
import { BrowserRouter, Navigate, Outlet, Route, Routes, useLocation } from 'react-router'
import AppShell from '@/components/AppShell'
import ToastRegion from '@/components/ToastRegion'
import { adminModules } from '@/modules'
import { AppProvider } from '@/stores/app'
import { AuthProvider, useAuth } from '@/stores/auth'
import { ToastProvider } from '@/stores/toast'
import LoginView from '@/views/LoginView'

function SessionBoundary() {
  const { initializationError, initialize, initialized } = useAuth()
  useEffect(() => { void initialize() }, [initialize])
  if (!initialized) {
    return <main className="app-loading" aria-label="Loading application"><span /></main>
  }
  if (initializationError) {
    return (
      <main className="login-page">
        <section className="login-panel initialization-panel" role="alert" aria-labelledby="initialization-title">
          <span className="brand-mark" aria-hidden="true">M</span>
          <h1 id="initialization-title">Application unavailable</h1>
          <p className="login-subtitle">{initializationError}</p>
          <button className="primary-button" type="button" onClick={() => void initialize()}>Retry</button>
        </section>
      </main>
    )
  }
  return <Outlet />
}

function RequireAuth() {
  const auth = useAuth()
  const location = useLocation()
  if (!auth.authenticated) {
    const next = `${location.pathname}${location.search}${location.hash}`
    return <Navigate to={`/login?next=${encodeURIComponent(next)}`} replace />
  }
  return <Outlet />
}

function LoginRoute() {
  const auth = useAuth()
  return auth.authenticated ? <Navigate to="/" replace /> : <LoginView />
}

export function AdminRoutes() {
  return (
    <Routes>
      <Route element={<SessionBoundary />}>
        <Route path="/login" element={<LoginRoute />} />
        <Route element={<RequireAuth />}>
          <Route path="/" element={<AppShell />}>
            <Route index element={<Navigate to={adminModules[0]?.path ?? '/login'} replace />} />
            {adminModules.map((module) => (
              <Route key={module.id} path={module.path.replace(/^\//, '')} element={<module.view />} />
            ))}
          </Route>
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}

export function AdminApp({ router = <BrowserRouter><AdminRoutes /></BrowserRouter> }: { router?: React.ReactNode }) {
  return (
    <AppProvider>
      <AuthProvider>
        <ToastProvider>
          {router}
          <ToastRegion />
        </ToastProvider>
      </AuthProvider>
    </AppProvider>
  )
}

export default function App() {
  return <AdminApp />
}
