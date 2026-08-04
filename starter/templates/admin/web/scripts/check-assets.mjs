import { spawnSync } from 'node:child_process'
import { existsSync, mkdtempSync, readdirSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, relative, resolve } from 'node:path'

const root = resolve(import.meta.dirname, '..')
const variants = existsSync(resolve(root, 'scripts/selections')) ? [
  ['default', '../internal/web/dist'],
  ['tasks', '../internal/web/dist-tasks'],
  ['audit', '../internal/web/dist-audit'],
  ['operations', '../internal/web/dist-operations'],
] : [[null, '../internal/web/dist']]

for (const [selection, output] of variants) {
  const expected = resolve(root, output)
  const temporary = mkdtempSync(join(tmpdir(), `modary-admin-${selection}-`))
  try {
    const result = spawnSync('pnpm', ['exec', 'vite', 'build', '--outDir', temporary, '--emptyOutDir'], {
      cwd: root,
      env: selection ? { ...process.env, VITE_ADMIN_SELECTION: selection } : process.env,
      encoding: 'utf8',
      stdio: 'pipe',
    })
    if (result.status !== 0) {
      process.stderr.write(result.stdout); process.stderr.write(result.stderr); process.exit(result.status ?? 1)
    }
    const expectedFiles = files(expected)
    const actualFiles = files(temporary)
    if (JSON.stringify(expectedFiles) !== JSON.stringify(actualFiles)) throw new Error(`${selection} asset file set differs`)
    for (const name of expectedFiles) {
      if (!readFileSync(join(expected, name)).equals(readFileSync(join(temporary, name)))) throw new Error(`${selection} asset differs: ${name}`)
    }
  } finally { rmSync(temporary, { recursive: true, force: true }) }
}
process.stdout.write('All Admin asset variants match their canonical production builds.\n')

function files(directory) { const result = []; walk(directory, directory, result); return result.sort() }
function walk(rootDirectory, directory, result) {
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const absolute = join(directory, entry.name)
    if (entry.isDirectory()) walk(rootDirectory, absolute, result)
    else result.push(relative(rootDirectory, absolute))
  }
}
