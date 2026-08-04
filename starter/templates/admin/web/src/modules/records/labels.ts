import type { RecordStatus } from './store'

export const recordStatusLabels: Readonly<Record<RecordStatus, string>> = {
  draft: '草稿',
  active: '启用',
  archived: '已归档',
}

export const recordFilterLabels: Readonly<Record<'all' | RecordStatus, string>> = {
  all: '全部',
  ...recordStatusLabels,
}
