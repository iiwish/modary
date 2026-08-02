// @vitest-environment node
import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const styles = readFileSync(new URL('./styles.css', import.meta.url), 'utf8')

describe('Admin visual tokens', () => {
  it('keeps normal muted text above the WCAG AA contrast threshold', () => {
    const foreground = cssColor('--muted')
    const background = cssColor('--canvas')
    expect(contrastRatio(foreground, background)).toBeGreaterThanOrEqual(4.5)
  })
})

function cssColor(name: string) {
  const match = styles.match(new RegExp(`${name}:\\s*(#[0-9a-f]{6})`, 'i'))
  if (!match?.[1]) throw new Error(`CSS token ${name} is missing or is not a six-digit hex color`)
  return match[1]
}

function contrastRatio(first: string, second: string) {
  const values = [relativeLuminance(first), relativeLuminance(second)].sort((left, right) => right - left)
  return ((values[0] ?? 0) + 0.05) / ((values[1] ?? 0) + 0.05)
}

function relativeLuminance(color: string) {
  const channels = [1, 3, 5].map((offset) => Number.parseInt(color.slice(offset, offset + 2), 16) / 255)
    .map((value) => value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4)
  return 0.2126 * (channels[0] ?? 0) + 0.7152 * (channels[1] ?? 0) + 0.0722 * (channels[2] ?? 0)
}
