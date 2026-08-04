// @vitest-environment node
import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

describe('Admin localization contract', () => {
  it('declares Simplified Chinese as the primary document language', () => {
    const html = readFileSync(new URL('../index.html', import.meta.url), 'utf8')
    expect(html).toContain('<html lang="zh-CN">')
    expect(html).toContain('<title>管理后台</title>')
  })
})
