import { useQuery } from '@tanstack/react-query'
import { ErrorState } from '@/components/states'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { formatNumber } from '@/lib/format'
import { type Format, fetchMessage, type Message } from './api'
import { CopyButton, PayloadView } from './payload-view'

type Props = {
  clusterId: string
  topic: string
  message: Message | null
  keyFormat: Format
  valueFormat: Format
  onClose: () => void
}

export function MessageSheet({ clusterId, topic, message, keyFormat, valueFormat, onClose }: Props) {
  // Streamed rows are capped in size; refetch the full record only when something was cut.
  const truncated =
    !!message && (message.key?.truncated || message.value?.truncated || message.headers.some((h) => h.value?.truncated))
  const full = useQuery({
    queryKey: ['message', clusterId, topic, message?.partition, message?.offset, keyFormat, valueFormat],
    queryFn: () => fetchMessage(clusterId, topic, message as Message, keyFormat, valueFormat),
    enabled: truncated,
    staleTime: Number.POSITIVE_INFINITY,
  })
  const m = truncated ? full.data : message

  return (
    <Sheet open={!!message} onOpenChange={(o) => !o && onClose()}>
      <SheetContent className="w-full overflow-y-auto data-[side=right]:sm:max-w-2xl">
        {message && (
          <>
            <SheetHeader>
              <SheetTitle className="font-mono">
                partition {message.partition} · offset {formatNumber(message.offset)}
              </SheetTitle>
              <SheetDescription>
                {new Date(message.timestamp).toISOString()} · {formatNumber(message.size)} bytes
              </SheetDescription>
            </SheetHeader>
            <div className="flex flex-col gap-6 px-4 pb-6">
              {truncated && full.error ? (
                <ErrorState error={full.error} title="Could not load the full message" />
              ) : !m ? (
                <Skeleton className="h-48 w-full" />
              ) : (
                <>
                  <PayloadView title="Key" payload={m.key} />
                  <PayloadView title="Value" payload={m.value} />
                  <section className="flex flex-col gap-2">
                    <div className="flex items-center">
                      <h3 className="font-medium text-sm">Headers ({m.headers.length})</h3>
                      <div className="ml-auto">
                        <CopyButton text={JSON.stringify(m, null, 2)} label="Copy as JSON" />
                      </div>
                    </div>
                    {m.headers.length > 0 && (
                      <div className="rounded-md border">
                        <Table>
                          <TableHeader>
                            <TableRow>
                              <TableHead>Key</TableHead>
                              <TableHead>Value</TableHead>
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {m.headers.map((h, i) => (
                              // Header keys may repeat, so the index is part of the key.
                              // biome-ignore lint/suspicious/noArrayIndexKey: headers are a static list
                              <TableRow key={`${h.key}-${i}`}>
                                <TableCell className="font-mono text-xs">{h.key}</TableCell>
                                <TableCell className="whitespace-pre-wrap break-all font-mono text-xs">
                                  {h.value ? h.value.text : <span className="text-muted-foreground">null</span>}
                                </TableCell>
                              </TableRow>
                            ))}
                          </TableBody>
                        </Table>
                      </div>
                    )}
                  </section>
                </>
              )}
            </div>
          </>
        )}
      </SheetContent>
    </Sheet>
  )
}
