import { useMemo } from 'react'
import { useAuth } from '@/stores/auth'
import type { AdminDescriptor } from '@/api/client'
import type { AdminModule, ResolvedAdminModule } from './types'

export function resolveAdminModules(
  selected: readonly AdminModule[],
  descriptors: readonly AdminDescriptor[],
  grants: ReadonlySet<string>,
): readonly ResolvedAdminModule[] {
  if (hasDuplicateIDs(selected) || hasDuplicateIDs(descriptors)) return []
  const available = new Map(selected.map((module) => [module.id, module]))
  return descriptors
    .filter((descriptor) => (descriptor.requiredPermissions ?? []).every((permission) => grants.has(permission)))
    .flatMap((descriptor) => {
      const module = available.get(descriptor.id)
      return module?.iconKey === descriptor.icon ? [{ ...module, label: descriptor.label, path: descriptor.path }] : []
    })
}

function hasDuplicateIDs(items: readonly { id: string }[]): boolean {
  return new Set(items.map(({ id }) => id)).size !== items.length
}

export function useAdminModules(selected: readonly AdminModule[]): readonly ResolvedAdminModule[] {
  const { grants, modules } = useAuth()
  return useMemo(() => resolveAdminModules(selected, modules, grants), [grants, modules, selected])
}
