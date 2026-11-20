/**
 * Pretty-prints JSON text without parsing it, so 64-bit integers and number
 * formatting are kept exactly as sent. Invalid input is returned unchanged.
 */
export function prettyJson(text: string, indent = 2): string {
  let out = ''
  let depth = 0
  let inString = false
  let escaped = false
  const pad = (d: number) => `\n${' '.repeat(d * indent)}`
  for (let i = 0; i < text.length; i++) {
    const ch = text[i]
    if (inString) {
      out += ch
      if (escaped) escaped = false
      else if (ch === '\\') escaped = true
      else if (ch === '"') inString = false
      continue
    }
    switch (ch) {
      case '"':
        inString = true
        out += ch
        break
      case '{':
      case '[': {
        // Keep empty containers on one line.
        let j = i + 1
        while (j < text.length && /\s/.test(text[j])) j++
        const close = ch === '{' ? '}' : ']'
        if (text[j] === close) {
          out += ch + close
          i = j
        } else {
          depth++
          out += ch + pad(depth)
        }
        break
      }
      case '}':
      case ']':
        depth = Math.max(0, depth - 1)
        out += pad(depth) + ch
        break
      case ',':
        out += `,${pad(depth)}`
        break
      case ':':
        out += ': '
        break
      case ' ':
      case '\n':
      case '\r':
      case '\t':
        break
      default:
        out += ch
    }
  }
  return inString || depth !== 0 ? text : out
}
