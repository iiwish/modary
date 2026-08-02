import { cleanup } from '@testing-library/react'
import { afterEach, vi } from 'vitest'
import { setAuthenticationExpiredHandler, setCSRFToken } from '@/api/client'

if (typeof HTMLDialogElement !== 'undefined' && !HTMLDialogElement.prototype.showModal) {
  HTMLDialogElement.prototype.showModal = function () { this.setAttribute('open', '') }
}
if (typeof HTMLDialogElement !== 'undefined' && !HTMLDialogElement.prototype.close) {
  HTMLDialogElement.prototype.close = function () { this.removeAttribute('open') }
}

afterEach(() => {
  cleanup()
  setCSRFToken('')
  setAuthenticationExpiredHandler(null)
  vi.unstubAllGlobals()
})
