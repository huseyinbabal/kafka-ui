export type DiffLine = { kind: 'same' | 'add' | 'del'; text: string; oldNo?: number; newNo?: number }

/**
 * Line diff via longest common subsequence. Fine for schema-sized inputs
 * (O(n·m) memory); inputs over maxLines fall back to a whole replace.
 */
export function diffLines(a: string, b: string, maxLines = 2000): DiffLine[] {
  const x = a.split('\n')
  const y = b.split('\n')
  if (x.length > maxLines || y.length > maxLines) {
    return [
      ...x.map((text, i) => ({ kind: 'del' as const, text, oldNo: i + 1 })),
      ...y.map((text, i) => ({ kind: 'add' as const, text, newNo: i + 1 })),
    ]
  }
  const n = x.length
  const m = y.length
  // lcs[i][j] = LCS length of x[i:] and y[j:]
  const lcs = Array.from({ length: n + 1 }, () => new Uint32Array(m + 1))
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      lcs[i][j] = x[i] === y[j] ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1])
    }
  }
  const out: DiffLine[] = []
  let i = 0
  let j = 0
  while (i < n && j < m) {
    if (x[i] === y[j]) {
      out.push({ kind: 'same', text: x[i], oldNo: i + 1, newNo: j + 1 })
      i++
      j++
    } else if (lcs[i + 1][j] >= lcs[i][j + 1]) {
      out.push({ kind: 'del', text: x[i], oldNo: i + 1 })
      i++
    } else {
      out.push({ kind: 'add', text: y[j], newNo: j + 1 })
      j++
    }
  }
  for (; i < n; i++) out.push({ kind: 'del', text: x[i], oldNo: i + 1 })
  for (; j < m; j++) out.push({ kind: 'add', text: y[j], newNo: j + 1 })
  return out
}
