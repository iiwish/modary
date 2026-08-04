import type { ComponentType } from 'react'
import type { LucideIcon } from 'lucide-react'

export type AdminModule = {
  id: string
  iconKey: string
  icon: LucideIcon
  view: ComponentType
}

export type ResolvedAdminModule = AdminModule & {
  label: string
  path: `/${string}`
}
