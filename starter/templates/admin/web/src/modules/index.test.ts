import { Files } from 'lucide-react'
import { describe, expect, it } from 'vitest'
import { resolveAdminModules } from './index'

const records = { id: 'records', iconKey: 'database', icon: Files, view: () => null }

describe('Admin contribution resolution', () => {
  it('fails closed for unknown, unauthorized, and incompletely granted modules', () => {
    const modules = resolveAdminModules([records], [
      { id: 'unknown', label: 'Unknown', path: '/unknown', icon: 'circle', order: 1 },
      { id: 'records', label: 'Records', path: '/records', icon: 'database', order: 2, requiredPermissions: ['records.list'] },
    ], new Set())
    expect(modules).toEqual([])
  })

  it('uses backend-owned labels and paths after permission filtering', () => {
    const modules = resolveAdminModules([records], [
      { id: 'records', label: 'Workspace records', path: '/records', icon: 'database', order: 2, requiredPermissions: ['records.list'] },
    ], new Set(['records.list']))
    expect(modules.map(({ id, label, path }) => ({ id, label, path }))).toEqual([
      { id: 'records', label: 'Workspace records', path: '/records' },
    ])
  })

  it('fails closed when selected module ids or icon contracts drift', () => {
    expect(resolveAdminModules([records], [
      { id: 'records', label: 'Records', path: '/records', icon: 'files', order: 1 },
    ], new Set())).toEqual([])
    expect(resolveAdminModules([records, { ...records, icon: Files }], [
      { id: 'records', label: 'Records', path: '/records', icon: 'database', order: 1 },
    ], new Set())).toEqual([])
    expect(resolveAdminModules([records], [
      { id: 'records', label: 'Records', path: '/records', icon: 'database', order: 1 },
      { id: 'records', label: 'Duplicate', path: '/duplicate', icon: 'database', order: 2 },
    ], new Set())).toEqual([])
  })
})
