import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { LogOut, Menu, PanelLeftClose, X } from 'lucide-react'
import { NavLink, Outlet, useNavigate } from 'react-router'
import { selectedAdminModules } from '@/modules/active'
import { useAdminModules } from '@/modules'
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
  const [menuFocusRequest, setMenuFocusRequest] = useState(0)
  const [pageFocusRequest, setPageFocusRequest] = useState(0)
  const adminModules = useAdminModules(selectedAdminModules)
  const menuButton = useRef<HTMLButtonElement>(null)
  const closeButton = useRef<HTMLButtonElement>(null)
  const sidebar = useRef<HTMLElement>(null)
  const workspace = useRef<HTMLElement>(null)

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

  useEffect(() => {
    if (!mobileViewport || !mobileOpen) return
    const previous = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => { document.body.style.overflow = previous }
  }, [mobileOpen, mobileViewport])

  useEffect(() => {
    if (pageFocusRequest === 0) return
    const frame = window.requestAnimationFrame(() => workspace.current?.querySelector<HTMLElement>('h1')?.focus())
    return () => window.cancelAnimationFrame(frame)
  }, [pageFocusRequest])

  useEffect(() => {
    if (menuFocusRequest === 0) return
    const frame = window.requestAnimationFrame(() => menuButton.current?.focus())
    return () => window.cancelAnimationFrame(frame)
  }, [menuFocusRequest])

  function closeMobileNavigation(restoreFocus = false) {
    setMobileOpen(false)
    if (restoreFocus) setMenuFocusRequest((value) => value + 1)
  }

  function handleSidebarKeyDown(event: KeyboardEvent<HTMLElement>) {
    if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      closeMobileNavigation(true)
      return
    }
    if (event.key !== 'Tab' || !mobileViewport || !mobileOpen) return
    const focusable = Array.from(sidebar.current?.querySelectorAll<HTMLElement>('a[href], button:not(:disabled):not([tabindex="-1"])') ?? [])
    const first = focusable[0]
    const last = focusable.at(-1)
    if (!first || !last) return
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault()
      last.focus()
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault()
      first.focus()
    }
  }

  function closeAfterSelection() {
    if (!mobileViewport) return
    setMobileOpen(false)
    setPageFocusRequest((value) => value + 1)
  }

  async function logout() {
    try {
      await auth.logout()
      await navigate('/login', { replace: true })
    } catch (cause) {
      toast.show(cause instanceof APIError ? cause.message : '退出登录失败，请重试。', 'error')
    }
  }

  const actorInitial = auth.actor?.display_name?.slice(0, 1) || 'A'
  return (
    <div className={`admin-layout${collapsed ? ' sidebar-collapsed' : ''}`}>
      <header className="mobile-header" inert={mobileViewport && mobileOpen}>
        <button ref={menuButton} className="icon-button" type="button" aria-label="打开导航" aria-controls="primary-sidebar" aria-expanded={mobileOpen} onClick={() => setMobileOpen(true)}>
          <Menu size={20} aria-hidden="true" />
        </button>
        <span className="mobile-brand">{app.name}</span>
        <span className="mobile-avatar" aria-hidden="true">{actorInitial}</span>
      </header>
      {mobileOpen && (
        <button className="nav-scrim" type="button" tabIndex={-1} aria-label="收起导航菜单" onClick={() => closeMobileNavigation(true)} />
      )}
      <aside
        ref={sidebar}
        id="primary-sidebar"
        className={`sidebar${mobileOpen ? ' mobile-open' : ''}`}
        aria-label="主导航"
        aria-hidden={mobileViewport && !mobileOpen ? true : undefined}
        aria-modal={mobileViewport && mobileOpen ? true : undefined}
        role={mobileViewport && mobileOpen ? 'dialog' : undefined}
        inert={mobileViewport && !mobileOpen}
        onKeyDown={handleSidebarKeyDown}
      >
        <div className="sidebar-brand">
          <span className="brand-mark small" aria-hidden="true">M</span>
          <span className="brand-copy">
            <strong>{app.name}</strong>
            <small>管理后台</small>
          </span>
          <button ref={closeButton} className="mobile-close icon-button" type="button" aria-label="关闭导航" onClick={() => closeMobileNavigation(true)}>
            <X size={19} aria-hidden="true" />
          </button>
        </div>
        <nav className="nav-list">
          {adminModules.map((module) => {
            const Icon = module.icon
            return (
              <NavLink key={module.id} to={module.path} onClick={closeAfterSelection}>
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
          <button className="icon-button" type="button" aria-label="退出登录" title="退出登录" disabled={auth.busy} onClick={() => void logout()}>
            <LogOut size={18} aria-hidden="true" />
          </button>
        </div>
        <button className="collapse-button" type="button" aria-label={collapsed ? '展开导航' : '收起导航'} aria-hidden={mobileViewport ? true : undefined} tabIndex={mobileViewport ? -1 : undefined} onClick={() => setCollapsed((value) => !value)}>
          <PanelLeftClose size={17} className={collapsed ? 'flipped' : undefined} aria-hidden="true" />
          <span>{collapsed ? '展开' : '收起'}</span>
        </button>
      </aside>
      <main ref={workspace} className="workspace" inert={mobileViewport && mobileOpen}>
        <Outlet />
      </main>
    </div>
  )
}
