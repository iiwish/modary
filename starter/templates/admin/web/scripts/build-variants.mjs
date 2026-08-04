import { spawnSync } from 'node:child_process'
import { existsSync } from 'node:fs'
import { resolve } from 'node:path'

const root = resolve(import.meta.dirname, '..')
const variants = existsSync(resolve(root, 'scripts/selections')) ? [
  ['default', '../internal/web/dist'],
  ['tasks', '../internal/web/dist-tasks'],
  ['audit', '../internal/web/dist-audit'],
  ['operations', '../internal/web/dist-operations'],
] : [[null, '../internal/web/dist']]

for (const [selection, output] of variants) {
  const result = spawnSync('pnpm', ['exec', 'vite', 'build', '--outDir', output, '--emptyOutDir'], {
    cwd: root,
    env: selection ? { ...process.env, VITE_ADMIN_SELECTION: selection } : process.env,
    encoding: 'utf8',
    stdio: 'inherit',
  })
  if (result.status !== 0) process.exit(result.status ?? 1)
}
