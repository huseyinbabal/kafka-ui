import { useMemo, useState } from 'react'
import { ErrorState, LoadingRows } from '@/components/states'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { formatBytes, formatNumber } from '@/lib/format'
import { RangePicker } from './analytics-page'
import { formatRate, type Range, useGroupSeries, useTopicSeries } from './api'
import { TimeChart } from './time-chart'

const span = (r: Range) => ({ '15m': 15, '1h': 60, '6h': 360, '24h': 1440, '7d': 10080 })[r] * 60_000

export function TopicThroughput({ clusterId, topic }: { clusterId: string; topic: string }) {
  const [range, setRange] = useState<Range>('1h')
  const { data, error, isPending } = useTopicSeries(clusterId, topic, range)
  const to = Date.now()
  const from = to - span(range)
  const msgs = useMemo(() => data?.points.map((p) => ({ ts: p.ts, rate: p.msgRate })) ?? [], [data])
  const size = useMemo(
    () => data?.points.filter((p) => p.sizeBytes >= 0).map((p) => ({ ts: p.ts, size: p.sizeBytes })) ?? [],
    [data],
  )
  return (
    <div className="flex flex-col gap-4">
      <div className="flex justify-end">
        <RangePicker value={range} onChange={setRange} />
      </div>
      {error ? (
        <ErrorState error={error} />
      ) : isPending ? (
        <LoadingRows rows={6} />
      ) : (
        <div className="grid gap-4 xl:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>Messages in / s</CardTitle>
            </CardHeader>
            <CardContent>
              <TimeChart
                data={msgs}
                series={[{ key: 'rate', label: 'msg/s' }]}
                from={from}
                to={to}
                format={(n) => `${formatRate(n)} msg/s`}
              />
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>Size</CardTitle>
              <CardDescription>Leader replicas, sampled every minute</CardDescription>
            </CardHeader>
            <CardContent>
              <TimeChart
                data={size}
                series={[{ key: 'size', label: 'size' }]}
                from={from}
                to={to}
                format={formatBytes}
              />
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  )
}

export function GroupLagTrend({ clusterId, group }: { clusterId: string; group: string }) {
  const [range, setRange] = useState<Range>('1h')
  const { data, error, isPending } = useGroupSeries(clusterId, group, range)
  const to = Date.now()
  const from = to - span(range)
  const lag = useMemo(() => data?.points.map((p) => ({ ts: p.ts, lag: p.lag })) ?? [], [data])
  const rate = useMemo(() => data?.points.map((p) => ({ ts: p.ts, rate: p.consumeRate })) ?? [], [data])
  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-center gap-3 space-y-0">
        <div className="mr-auto">
          <CardTitle>Lag trend</CardTitle>
          <CardDescription>Total lag and consume rate of this group over time</CardDescription>
        </div>
        <RangePicker value={range} onChange={setRange} />
      </CardHeader>
      <CardContent>
        {error ? (
          <ErrorState error={error} />
        ) : isPending ? (
          <LoadingRows rows={4} />
        ) : (
          <div className="grid gap-4 xl:grid-cols-2">
            <TimeChart
              data={lag}
              series={[{ key: 'lag', label: 'lag' }]}
              from={from}
              to={to}
              format={(n) => `${formatNumber(n)} messages`}
            />
            <TimeChart
              data={rate}
              series={[{ key: 'rate', label: 'consumed/s' }]}
              from={from}
              to={to}
              format={(n) => `${formatRate(n)} msg/s`}
            />
          </div>
        )}
      </CardContent>
    </Card>
  )
}
