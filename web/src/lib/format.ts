const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']

export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return '—'
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v < 10 && i > 0 ? v.toFixed(1) : Math.round(v)} ${units[i]}`
}

const nf = new Intl.NumberFormat('en-US')
export function formatNumber(n: number): string {
  if (!Number.isFinite(n)) return '—'
  return nf.format(n)
}

const cf = new Intl.NumberFormat('en-US', { notation: 'compact', maximumFractionDigits: 1 })
export function formatCompact(n: number): string {
  if (!Number.isFinite(n)) return '—'
  return cf.format(n)
}

/** Offsets use -1 as "none". */
export function formatOffset(n: number): string {
  return n < 0 ? '—' : nf.format(n)
}
