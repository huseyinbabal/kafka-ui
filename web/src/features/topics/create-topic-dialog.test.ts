import { describe, expect, it } from 'vitest'
import { parseConfigLines } from './create-topic-dialog'

describe('parseConfigLines', () => {
  it('parses key=value lines and skips comments', () => {
    expect(parseConfigLines('# c\n a = 1 \n\nb=x=y')).toEqual({ configs: { a: '1', b: 'x=y' } })
  })
  it('reports the bad line', () => {
    expect(parseConfigLines('a=1\nnope').error).toBe('line 2: expected key=value')
  })
})
