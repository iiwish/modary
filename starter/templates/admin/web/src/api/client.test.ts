import { describe, expect, it, vi } from 'vitest'
import { api } from './client'

describe('API errors', () => {
  it('localizes stable platform error codes for the Chinese Admin UI', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({
      error: { code: 'AUTHENTICATION_FAILED', message: 'credentials are invalid' },
      request_id: 'req_localized',
    }), { status: 401 })))

    await expect(api('/api/auth/login')).rejects.toMatchObject({
      code: 'AUTHENTICATION_FAILED',
      message: '用户名或密码错误。',
      requestID: 'req_localized',
    })
  })

  it('preserves server detail for unknown application error codes', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({
      error: { code: 'DOMAIN_RULE_FAILED', message: '业务规则不允许此操作。' },
    }), { status: 422 })))

    await expect(api('/api/domain')).rejects.toMatchObject({
      code: 'DOMAIN_RULE_FAILED',
      message: '业务规则不允许此操作。',
    })
  })
})
