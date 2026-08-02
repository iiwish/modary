import { describe, expect, it } from 'vitest'
import { adminModules } from './index'

describe('Admin module registry', () => {
  it('contains only the selected records module', () => {
    expect(adminModules.map(({ id, path }) => ({ id, path }))).toEqual([{ id: 'records', path: '/records' }])
    expect(JSON.stringify(adminModules)).not.toMatch(/tasks|audit|actions|mcp|marketplace/i)
  })
})
