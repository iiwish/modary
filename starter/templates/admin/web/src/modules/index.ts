import { recordsModule } from './records'
import type { AdminModule } from './types'

// This explicit source registry is the complete selected Admin UI surface.
export const adminModules: readonly AdminModule[] = [recordsModule]
