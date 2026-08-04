import { spawnSync } from 'node:child_process'
import { existsSync } from 'node:fs'
import { resolve } from 'node:path'

const root = resolve(import.meta.dirname, '..')
const variants = existsSync(resolve(root, 'scripts/selections')) ? [
  ['default', 'local', '../internal/web/dist'],
  ['tasks', 'local', '../internal/web/dist-tasks'],
  ['audit', 'local', '../internal/web/dist-audit'],
  ['operations', 'local', '../internal/web/dist-operations'],
  ['default', 'oidc', '../internal/web/dist-oidc'],
  ['tasks', 'oidc', '../internal/web/dist-oidc-tasks'],
  ['audit', 'oidc', '../internal/web/dist-oidc-audit'],
  ['operations', 'oidc', '../internal/web/dist-oidc-operations'],
] : [[null, 'local', '../internal/web/dist']]

for (const [selection, authentication, output] of variants) {
  const result = spawnSync('pnpm', ['exec', 'vite', 'build', '--outDir', output, '--emptyOutDir'], {
    cwd: root,
    env: selection ? { ...process.env, VITE_ADMIN_SELECTION: selection, VITE_AUTH_SELECTION: authentication } : process.env,
    encoding: 'utf8',
    stdio: 'inherit',
  })
  if (result.status !== 0) process.exit(result.status ?? 1)
}
