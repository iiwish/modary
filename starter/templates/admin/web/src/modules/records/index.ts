import { Database } from 'lucide-react'
import RecordsView from './RecordsView'
import type { AdminModule } from '../types'

export const recordsModule: AdminModule = {
  id: 'records',
  iconKey: 'database',
  icon: Database,
  view: RecordsView
}
