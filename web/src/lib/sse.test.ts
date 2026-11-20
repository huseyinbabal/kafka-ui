import { describe, expect, it } from 'vitest'
import { SSEParser } from './sse'

describe('SSEParser', () => {
  it('parses events split across chunks', () => {
    const p = new SSEParser()
    expect(p.push('event: message\ndata: {"a"')).toEqual([])
    expect(p.push(':1}\n\nevent: done\r\ndata: x\r\n\r\n')).toEqual([
      { event: 'message', data: '{"a":1}' },
      { event: 'done', data: 'x' },
    ])
  })

  it('ignores comments and joins multi-line data', () => {
    const p = new SSEParser()
    expect(p.push(': ping\n\ndata: a\ndata: b\n\n')).toEqual([{ event: 'message', data: 'a\nb' }])
  })
})
