import { render, screen } from '@testing-library/react'
import axe from 'axe-core'
import { MemoryRouter } from 'react-router'
import { describe, expect, it } from 'vitest'
import { AppProvider } from '@/stores/app'
import { AuthProvider } from '@/stores/auth'
import LoginView from './LoginView'

describe('Login view', () => {
  it('has an accessible keyboard form', async () => {
    const { container } = render(
      <AppProvider><AuthProvider><MemoryRouter><LoginView /></MemoryRouter></AuthProvider></AppProvider>,
    )
    expect(screen.getByLabelText('用户名').tagName).toBe('INPUT')
    expect(screen.getByLabelText('密码').getAttribute('type')).toBe('password')
    const result = await axe.run(container)
    expect(result.violations).toEqual([])
  })
})
