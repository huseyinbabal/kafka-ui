import { Link, useParams } from '@tanstack/react-router'
import { AlertTriangleIcon, CrownIcon, DatabaseIcon, LayersIcon, ListTreeIcon } from 'lucide-react'
import { PageHeader, StatCard } from '@/components/page'
import { ErrorState, LoadingRows } from '@/components/states'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { Broker } from '@/lib/api'
import { formatBytes, formatNumber } from '@/lib/format'
import { useCluster, useOverview } from './api'

export function OverviewPage() {
  const { clusterId } = useParams({ from: '/clusters/$clusterId/' })
  const { data: cluster } = useCluster(clusterId)
  const { data, isPending, error } = useOverview(clusterId)

  return (
    <>
      <PageHeader
        title={cluster?.name ?? clusterId}
        description={
          data && (
            <span className="font-mono text-xs">
              cluster id {data.clusterId} · Kafka {data.version || 'unknown'}
            </span>
          )
        }
        actions={cluster?.readOnly && <Badge variant="secondary">read-only</Badge>}
      />
      {error ? (
        <ErrorState title="Could not load cluster" error={error} />
      ) : isPending ? (
        <LoadingRows rows={6} />
      ) : (
        <>
          <div className="grid grid-cols-2 gap-4 lg:grid-cols-5">
            <StatCard label="Brokers" value={data.brokers.length} icon={<DatabaseIcon className="size-4" />} />
            <StatCard
              label="Topics"
              value={formatNumber(data.topicCount)}
              hint={`${data.internalTopicCount} internal`}
              icon={<ListTreeIcon className="size-4" />}
            />
            <StatCard
              label="Partitions"
              value={formatNumber(data.partitionCount)}
              icon={<LayersIcon className="size-4" />}
            />
            <StatCard
              label="Under-replicated"
              value={data.underReplicatedPartitions}
              tone={data.underReplicatedPartitions > 0 ? 'warn' : 'ok'}
              hint={`${data.offlinePartitions} offline`}
              icon={<AlertTriangleIcon className="size-4" />}
            />
            <StatCard label="Total size" value={formatBytes(data.totalSizeBytes)} hint="all replicas" />
          </div>
          <BrokersCard clusterId={clusterId} brokers={data.brokers} />
        </>
      )}
    </>
  )
}

export function BrokersCard({ clusterId, brokers }: { clusterId: string; brokers: Broker[] }) {
  const maxLeaders = Math.max(1, ...brokers.map((b) => b.leaderCount))
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center justify-between">
          Brokers
          <Link
            to="/clusters/$clusterId/brokers"
            params={{ clusterId }}
            className="font-normal text-muted-foreground text-sm hover:underline"
          >
            details
          </Link>
        </CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>ID</TableHead>
              <TableHead>Host</TableHead>
              <TableHead>Rack</TableHead>
              <TableHead>Leader share</TableHead>
              <TableHead className="text-right">Replicas</TableHead>
              <TableHead className="text-right">Size</TableHead>
              <TableHead className="text-right">Disk free</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {brokers.map((b) => (
              <TableRow key={b.id}>
                <TableCell className="font-medium">
                  <span className="inline-flex items-center gap-1.5">
                    {b.id}
                    {b.isController && (
                      <Badge variant="secondary" className="gap-1">
                        <CrownIcon className="size-3" />
                        controller
                      </Badge>
                    )}
                  </span>
                </TableCell>
                <TableCell className="max-w-72 truncate font-mono text-xs">
                  {b.host}:{b.port}
                </TableCell>
                <TableCell>{b.rack ?? '—'}</TableCell>
                <TableCell>
                  <div className="flex items-center gap-2">
                    <div className="h-1.5 w-24 overflow-hidden rounded-full bg-muted">
                      <div
                        className="h-full rounded-full bg-primary"
                        style={{ width: `${(b.leaderCount / maxLeaders) * 100}%` }}
                      />
                    </div>
                    <span className="text-xs tabular-nums">{b.leaderCount}</span>
                  </div>
                </TableCell>
                <TableCell className="text-right tabular-nums">{b.replicaCount}</TableCell>
                <TableCell className="text-right tabular-nums">{formatBytes(b.sizeBytes)}</TableCell>
                <TableCell className="text-right tabular-nums">
                  {b.diskTotalBytes > 0 ? `${formatBytes(b.diskUsableBytes)} / ${formatBytes(b.diskTotalBytes)}` : '—'}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}
