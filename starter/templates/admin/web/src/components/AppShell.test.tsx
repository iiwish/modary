import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router'
import { describe, expect, it, vi } from 'vitest'
import { AppProvider } from '@/stores/app'
import { AuthProvider } from '@/stores/auth'
import { ToastProvider } from '@/stores/toast'
import AppShell from './AppShell'

describe('Admin shell', () => {
  it('keeps closed mobile navigation inert and restores menu focus after Escape', async () => {
    vi.stubGlobal('matchMedia', vi.fn(() => ({
      matches: true,
      media: '(max-width: 800px)',
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })))
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => { callback(0); return 1 })
    const user = userEvent.setup()
    render(
      <AppProvider><AuthProvider><ToastProvider>
        <MemoryRouter initialEntries={['/records']}>
          <Routes><Route path="/" element={<AppShell />}><Route path="records" element={<div>Records</div>} /></Route></Routes>
        </MemoryRouter>
      </ToastProvider></AuthProvider></AppProvider>,
    )

    const sidebar = document.getElementById('primary-sidebar')
    if (!sidebar) throw new Error('Primary navigation was not rendered')
    expect(sidebar.getAttribute('aria-hidden')).toBe('true')
    expect(sidebar.hasAttribute('inert')).toBe(true)

    const menu = screen.getByRole('button', { name: 'Open navigation' })
    await user.click(menu)
    expect(sidebar.getAttribute('aria-hidden')).toBeNull()
    expect(document.activeElement?.getAttribute('aria-label')).toBe('Close navigation')

    fireEvent.keyDown(sidebar, { key: 'Escape' })
    await waitFor(() => expect(document.activeElement).toBe(menu))
    expect(sidebar.getAttribute('aria-hidden')).toBe('true')
  })
})
