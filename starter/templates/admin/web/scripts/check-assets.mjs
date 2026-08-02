import { spawnSync } from 'node:child_process'
import { mkdtempSync, readdirSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, relative, resolve } from 'node:path'

const root = resolve(import.meta.dirname, '..')
const expected = resolve(root, '../internal/web/dist')
const temporary = mkdtempSync(join(tmpdir(), 'modary-admin-assets-'))

try {
  const result = spawnSync('pnpm', ['exec', 'vite', 'build', '--outDir', temporary, '--emptyOutDir'], {
    cwd: root,
    encoding: 'utf8',
    stdio: 'pipe'
  })
  if (result.status !== 0) {
    process.stderr.write(result.stdout)
    process.stderr.write(result.stderr)
    process.exit(result.status ?? 1)
  }
  const expectedFiles = files(expected)
  const actualFiles = files(temporary)
  if (JSON.stringify(expectedFiles) !== JSON.stringify(actualFiles)) {
    throw new Error(`asset file set differs: expected ${expectedFiles.join(', ')}, built ${actualFiles.join(', ')}`)
  }
  for (const name of expectedFiles) {
    if (!readFileSync(join(expected, name)).equals(readFileSync(join(temporary, name)))) {
      throw new Error(`built asset differs: ${name}`)
    }
  }
  process.stdout.write('Admin assets match the canonical production build.\n')
} finally {
  rmSync(temporary, { recursive: true, force: true })
}

function files(directory) {
  const result = []
  walk(directory, directory, result)
  return result.sort()
}

function walk(rootDirectory, directory, result) {
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const absolute = join(directory, entry.name)
    if (entry.isDirectory()) walk(rootDirectory, absolute, result)
    else result.push(relative(rootDirectory, absolute))
  }
}
