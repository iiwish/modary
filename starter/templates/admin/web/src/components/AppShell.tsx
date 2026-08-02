import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { LogOut, Menu, PanelLeftClose, X } from 'lucide-react'
import { NavLink, Outlet, useNavigate } from 'react-router'
import { adminModules } from '@/modules'
import { APIError } from '@/api/client'
import { useApp } from '@/stores/app'
import { useAuth } from '@/stores/auth'
import { useToast } from '@/stores/toast'

export default function AppShell() {
  const app = useApp()
  const auth = useAuth()
  const toast = useToast()
  const navigate = useNavigate()
  const [mobileOpen, setMobileOpen] = useState(false)
  const [collapsed, setCollapsed] = useState(false)
  const [mobileViewport, setMobileViewport] = useState(false)
  const menuButton = useRef<HTMLButtonElement>(null)
  const closeButton = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    const query = window.matchMedia('(max-width: 800px)')
    const sync = (event: MediaQueryList | MediaQueryListEvent) => {
      setMobileViewport(event.matches)
      if (!event.matches) setMobileOpen(false)
    }
    sync(query)
    query.addEventListener('change', sync)
    return () => query.removeEventListener('change', sync)
  }, [])

  useEffect(() => {
    if (mobileOpen) closeButton.current?.focus()
  }, [mobileOpen])

  function closeMobileNavigation(restoreFocus = false) {
    setMobileOpen(false)
    if (restoreFocus) window.requestAnimationFrame(() => menuButton.current?.focus())
  }

  function handleSidebarKeyDown(event: KeyboardEvent<HTMLElement>) {
    if (event.key !== 'Escape') return
    event.preventDefault()
    event.stopPropagation()
    closeMobileNavigation(true)
  }

  async function logout() {
    try {
      await auth.logout()
      await navigate('/login', { replace: true })
    } catch (cause) {
      toast.show(cause instanceof APIError ? cause.message : 'Could not sign out. Try again.', 'error')
    }
  }

  const actorInitial = auth.actor?.display_name?.slice(0, 1) || 'A'
  return (
    <div className={`admin-layout${collapsed ? ' sidebar-collapsed' : ''}`}>
      <header className="mobile-header">
        <button ref={menuButton} className="icon-button" type="button" aria-label="Open navigation" aria-controls="primary-sidebar" aria-expanded={mobileOpen} onClick={() => setMobileOpen(true)}>
          <Menu size={20} aria-hidden="true" />
        </button>
        <span className="mobile-brand">{app.name}</span>
        <span className="mobile-avatar" aria-hidden="true">{actorInitial}</span>
      </header>
      {mobileOpen && (
        <button className="nav-scrim" type="button" tabIndex={-1} aria-label="Dismiss navigation" onClick={() => closeMobileNavigation(true)} />
      )}
      <aside
        id="primary-sidebar"
        className={`sidebar${mobileOpen ? ' mobile-open' : ''}`}
        aria-label="Primary navigation"
        aria-hidden={mobileViewport && !mobileOpen ? true : undefined}
        inert={mobileViewport && !mobileOpen}
        onKeyDown={handleSidebarKeyDown}
      >
        <div className="sidebar-brand">
          <span className="brand-mark small" aria-hidden="true">M</span>
          <span className="brand-copy">
            <strong>{app.name}</strong>
            <small>Administration</small>
          </span>
          <button ref={closeButton} className="mobile-close icon-button" type="button" aria-label="Close navigation" onClick={() => closeMobileNavigation(true)}>
            <X size={19} aria-hidden="true" />
          </button>
        </div>
        <nav className="nav-list">
          {adminModules.map((module) => {
            const Icon = module.icon
            return (
              <NavLink key={module.id} to={module.path} onClick={() => closeMobileNavigation()}>
                <Icon size={18} aria-hidden="true" />
                <span>{module.label}</span>
              </NavLink>
            )
          })}
        </nav>
        <div className="sidebar-footer">
          <div className="account-summary">
            <span className="avatar" aria-hidden="true">{actorInitial}</span>
            <span className="account-copy"><strong>{auth.actor?.display_name}</strong><small>{auth.actor?.type}</small></span>
          </div>
          <button className="icon-button" type="button" aria-label="Sign out" title="Sign out" disabled={auth.busy} onClick={() => void logout()}>
            <LogOut size={18} aria-hidden="true" />
          </button>
        </div>
        <button className="collapse-button" type="button" aria-label={collapsed ? 'Expand navigation' : 'Collapse navigation'} onClick={() => setCollapsed((value) => !value)}>
          <PanelLeftClose size={17} className={collapsed ? 'flipped' : undefined} aria-hidden="true" />
          <span>{collapsed ? 'Expand' : 'Collapse'}</span>
        </button>
      </aside>
      <main className="workspace">
        <Outlet />
      </main>
    </div>
  )
}
