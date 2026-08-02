import type { ComponentType } from 'react'
import type { LucideIcon } from 'lucide-react'

export type AdminModule = {
  id: string
  label: string
  path: `/${string}`
  icon: LucideIcon
  view: ComponentType
}
