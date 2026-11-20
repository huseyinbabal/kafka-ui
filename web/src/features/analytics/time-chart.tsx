import { Area, AreaChart, CartesianGrid, Line, LineChart, XAxis, YAxis } from 'recharts'
import {
  type ChartConfig,
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
} from '@/components/ui/chart'
import { formatCompact } from '@/lib/format'
import { cn } from '@/lib/utils'

export type Series = { key: string; label: string }

/** Series take categorical slots in order; the palette is validated for three. */
const slot = (i: number) => `var(--series-${(i % 3) + 1})`

type Props = {
  data: Record<string, number>[]
  series: Series[]
  from: number
  to: number
  format: (n: number) => string
  className?: string
}

function tickTime(from: number, to: number) {
  const long = to - from > 26 * 3600_000
  return (ts: number) =>
    new Date(ts).toLocaleString(
      undefined,
      long ? { month: 'short', day: 'numeric' } : { hour: '2-digit', minute: '2-digit' },
    )
}

/**
 * One y-axis time series. A single series renders as an area; several render
 * as lines with a legend (identity is never color alone).
 */
export function TimeChart({ data, series, from, to, format, className }: Props) {
  const config: ChartConfig = Object.fromEntries(series.map((s, i) => [s.key, { label: s.label, color: slot(i) }]))
  const single = series.length === 1
  const tick = tickTime(from, to)
  const common = (
    <>
      <CartesianGrid vertical={false} />
      <XAxis
        dataKey="ts"
        type="number"
        scale="time"
        domain={[from, to]}
        tickFormatter={tick}
        tickLine={false}
        axisLine={false}
        minTickGap={48}
      />
      <YAxis
        tickFormatter={(n: number) => formatCompact(n)}
        tickLine={false}
        axisLine={false}
        width={44}
        allowDecimals={false}
      />
      <ChartTooltip
        content={
          <ChartTooltipContent
            indicator="line"
            labelFormatter={(_, payload) => {
              const ts = payload?.[0]?.payload?.ts
              return ts ? new Date(ts).toLocaleString() : ''
            }}
            formatter={(value, name) => (
              <div className="flex w-full items-center justify-between gap-4">
                <span className="text-muted-foreground">{config[String(name)]?.label ?? name}</span>
                <span className="font-medium font-mono tabular-nums">{format(Number(value))}</span>
              </div>
            )}
          />
        }
      />
      {!single && <ChartLegend content={<ChartLegendContent />} />}
    </>
  )
  return (
    <ChartContainer config={config} className={cn('aspect-auto h-56 w-full', className)}>
      {single ? (
        <AreaChart data={data} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
          {common}
          <Area
            dataKey={series[0].key}
            type="monotone"
            stroke={`var(--color-${series[0].key})`}
            strokeWidth={2}
            fill={`var(--color-${series[0].key})`}
            fillOpacity={0.15}
            isAnimationActive={false}
            connectNulls={false}
          />
        </AreaChart>
      ) : (
        <LineChart data={data} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
          {common}
          {series.map((s) => (
            <Line
              key={s.key}
              dataKey={s.key}
              type="monotone"
              stroke={`var(--color-${s.key})`}
              strokeWidth={2}
              dot={false}
              isAnimationActive={false}
            />
          ))}
        </LineChart>
      )}
    </ChartContainer>
  )
}
