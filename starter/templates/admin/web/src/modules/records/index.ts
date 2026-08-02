import { Files } from 'lucide-react'
import RecordsView from './RecordsView'
import type { AdminModule } from '../types'

export const recordsModule: AdminModule = {
  id: 'records',
  label: 'Records',
  path: '/records',
  icon: Files,
  view: RecordsView
}
