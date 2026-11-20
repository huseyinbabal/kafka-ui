import { describe, expect, it } from 'vitest'
import { prettyJson } from './json'

describe('prettyJson', () => {
  it('indents objects and arrays', () => {
    expect(prettyJson('{"a":1,"b":[true,null],"c":{}}')).toBe(
      '{\n  "a": 1,\n  "b": [\n    true,\n    null\n  ],\n  "c": {}\n}',
    )
  })
  it('keeps big integers exactly', () => {
    expect(prettyJson('{"id":12345678901234567890}')).toContain('12345678901234567890')
  })
  it('does not touch structural characters inside strings', () => {
    expect(prettyJson('{"s":"a, {b}: \\"c\\""}')).toBe('{\n  "s": "a, {b}: \\"c\\""\n}')
  })
  it('returns invalid input unchanged', () => {
    expect(prettyJson('{"a":')).toBe('{"a":')
  })
})
