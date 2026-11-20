import { describe, expect, it } from 'vitest'
import { diffLines } from './diff'

describe('diffLines', () => {
  it('marks added and removed lines', () => {
    const d = diffLines('a\nb\nc', 'a\nc\nd')
    expect(d.map((l) => `${l.kind}:${l.text}`)).toEqual(['same:a', 'del:b', 'same:c', 'add:d'])
    expect(d[2]).toMatchObject({ oldNo: 3, newNo: 2 })
  })
  it('handles identical and empty inputs', () => {
    expect(diffLines('x', 'x').every((l) => l.kind === 'same')).toBe(true)
    expect(diffLines('', 'y').map((l) => l.kind)).toEqual(['del', 'add'])
  })
})
