import { describe, expect, it } from 'vitest'
import { formatDuration, formatRate } from './api'

describe('formatDuration', () => {
  it('uses the two largest units', () => {
    expect(formatDuration(45)).toBe('45s')
    expect(formatDuration(150)).toBe('2m 30s')
    expect(formatDuration(3900)).toBe('1h 5m')
    expect(formatDuration(3 * 86400 + 4 * 3600 + 59)).toBe('3d 4h')
  })
})

describe('formatRate', () => {
  it('keeps one decimal for small rates', () => {
    expect(formatRate(0)).toBe('0')
    expect(formatRate(2.345)).toBe('2.3')
    expect(formatRate(1234.6)).toBe('1,235')
  })
})

import { hotPartitions } from './analytics-page'

describe('hotPartitions', () => {
  it('flags partitions over 1.5x their fair share', () => {
    const hot = hotPartitions([
      { topic: 't', partition: 0, leader: 1, rate: 70 },
      { topic: 't', partition: 1, leader: 2, rate: 10 },
      { topic: 't', partition: 2, leader: 0, rate: 10 },
      { topic: 't', partition: 3, leader: 1, rate: 10 },
      { topic: 'single', partition: 0, leader: 1, rate: 100 },
      { topic: 'quiet', partition: 0, leader: 1, rate: 0 },
      { topic: 'quiet', partition: 1, leader: 1, rate: 0 },
    ])
    expect(hot).toHaveLength(1)
    expect(hot[0]).toMatchObject({ topic: 't', partition: 0, share: 0.7, partitions: 4 })
  })
})
