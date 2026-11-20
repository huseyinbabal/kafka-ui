import { ApiError } from './api'

export type SSEEvent = { event: string; data: string }

/**
 * Incremental Server-Sent Events parser. Feed it text chunks; it returns the
 * complete events found so far and keeps the remainder buffered.
 */
export class SSEParser {
  private buf = ''

  push(chunk: string): SSEEvent[] {
    this.buf += chunk.replace(/\r\n?/g, '\n')
    const out: SSEEvent[] = []
    let idx = this.buf.indexOf('\n\n')
    while (idx >= 0) {
      const block = this.buf.slice(0, idx)
      this.buf = this.buf.slice(idx + 2)
      let event = 'message'
      const data: string[] = []
      for (const line of block.split('\n')) {
        if (line === '' || line.startsWith(':')) continue
        const colon = line.indexOf(':')
        const field = colon < 0 ? line : line.slice(0, colon)
        let value = colon < 0 ? '' : line.slice(colon + 1)
        if (value.startsWith(' ')) value = value.slice(1)
        if (field === 'event') event = value
        else if (field === 'data') data.push(value)
      }
      if (data.length > 0) out.push({ event, data: data.join('\n') })
      idx = this.buf.indexOf('\n\n')
    }
    return out
  }
}

/**
 * Opens an SSE stream with fetch (so errors before the stream starts come back
 * as JSON and the request can be aborted). Resolves when the stream ends.
 */
export async function openSSE(path: string, onEvent: (e: SSEEvent) => void, signal: AbortSignal): Promise<void> {
  const res = await fetch(`/api/v1${path}`, {
    headers: { Accept: 'text/event-stream' },
    credentials: 'same-origin',
    signal,
  })
  if (!res.ok || !res.body) {
    const data = await res.json().catch(() => undefined)
    throw new ApiError(res.status, data?.error?.code ?? 'UNKNOWN', data?.error?.message ?? res.statusText)
  }
  const reader = res.body.pipeThrough(new TextDecoderStream()).getReader()
  const parser = new SSEParser()
  for (;;) {
    const { value, done } = await reader.read()
    if (done) return
    for (const e of parser.push(value)) onEvent(e)
  }
}
