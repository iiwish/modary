import { ListTodo } from 'lucide-react'
import type { AdminModule } from '../types'
import TasksView from './TasksView'

export const tasksModule: AdminModule = { id: 'tasks', iconKey: 'list-todo', icon: ListTodo, view: TasksView }
