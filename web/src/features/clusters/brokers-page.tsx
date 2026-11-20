import { useQuery } from '@tanstack/react-query'
import { useParams } from '@tanstack/react-router'
import { useState } from 'react'
import { ConfigTable } from '@/components/config-table'
import { PageHeader } from '@/components/page'
import { ErrorState, LoadingRows } from '@/components/states'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { type ConfigEntry, enc, get, type List } from '@/lib/api'
import { useOverview } from './api'
import { BrokersCard } from './overview-page'

export function BrokersPage() {
  const { clusterId } = useParams({ from: '/clusters/$clusterId/brokers' })
  const { data, isPending, error } = useOverview(clusterId)
  const [selected, setSelected] = useState<number | null>(null)
  const brokerId = selected ?? data?.brokers[0]?.id ?? null

  const configs = useQuery({
    queryKey: ['clusters', clusterId, 'brokers', brokerId, 'configs'],
    queryFn: () => get<List<ConfigEntry>>(`/clusters/${enc(clusterId)}/brokers/${brokerId}/configs`),
    enabled: brokerId !== null,
    select: (d) => d.items,
  })

  if (error) return <ErrorState error={error} />
  if (isPending) return <LoadingRows rows={6} />

  return (
    <>
      <PageHeader title="Brokers" description={`${data.brokers.length} brokers · controller ${data.controllerId}`} />
      <BrokersCard clusterId={clusterId} brokers={data.brokers} />
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center justify-between gap-4">
            Broker configuration
            <Select value={brokerId === null ? '' : String(brokerId)} onValueChange={(v) => setSelected(Number(v))}>
              <SelectTrigger className="w-40">
                <SelectValue placeholder="Broker" />
              </SelectTrigger>
              <SelectContent>
                {data.brokers.map((b) => (
                  <SelectItem key={b.id} value={String(b.id)}>
                    Broker {b.id}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </CardTitle>
        </CardHeader>
        <CardContent>
          {configs.error ? (
            <ErrorState error={configs.error} />
          ) : (
            <ConfigTable configs={configs.data ?? []} loading={configs.isPending} />
          )}
        </CardContent>
      </Card>
    </>
  )
}
