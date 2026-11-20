import { describe, expect, it } from 'vitest'
import { formatBytes, formatCompact, formatOffset } from './format'

describe('formatBytes', () => {
  it.each([
    [0, '0 B'],
    [512, '512 B'],
    [1536, '1.5 KB'],
    [10 * 1024 * 1024, '10 MB'],
    [-1, '—'],
  ])('%d -> %s', (input, want) => {
    expect(formatBytes(input)).toBe(want)
  })
})

describe('formatOffset', () => {
  it('renders -1 as dash', () => expect(formatOffset(-1)).toBe('—'))
  it('groups thousands', () => expect(formatOffset(1234567)).toBe('1,234,567'))
})

describe('formatCompact', () => {
  it('compacts', () => expect(formatCompact(12_300)).toBe('12.3K'))
})
