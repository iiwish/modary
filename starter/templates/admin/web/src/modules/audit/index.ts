import { History } from 'lucide-react'
import type { AdminModule } from '../types'
import AuditView from './AuditView'

export const auditModule: AdminModule = { id: 'audit', iconKey: 'history', icon: History, view: AuditView }
