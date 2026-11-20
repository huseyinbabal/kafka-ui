import { Link, useNavigate, useParams } from '@tanstack/react-router'
import { ChevronLeftIcon, Trash2Icon } from 'lucide-react'
import { useMemo, useState } from 'react'
import { toast } from 'sonner'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { PageHeader } from '@/components/page'
import { ErrorState, LoadingRows } from '@/components/states'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useCanMutate } from '@/features/auth/api'
import { CopyButton } from '@/features/messages/payload-view'
import { diffLines } from '@/lib/diff'
import { cn } from '@/lib/utils'
import {
  compatibilityLevels,
  formatSchema,
  type SchemaVersion,
  useDeleteSubject,
  useSetCompatibility,
  useSubject,
} from './api'
import { RegisterSchemaDialog } from './register-dialog'
import { SchemaTypeBadge } from './schemas-page'

const inherit = '__inherit__'

function SchemaDiff({ from, to }: { from: SchemaVersion; to: SchemaVersion }) {
  const lines = useMemo(() => diffLines(formatSchema(from), formatSchema(to)), [from, to])
  const changed = lines.filter((l) => l.kind !== 'same').length
  return (
    <div className="flex flex-col gap-2">
      <p className="text-muted-foreground text-xs">
        v{from.version} → v{to.version}: {changed === 0 ? 'identical' : `${changed} changed lines`}
      </p>
      <pre className="max-h-[60vh] overflow-auto rounded-md border bg-muted/40 py-2 font-mono text-xs">
        {lines.map((l, i) => (
          <div
            // biome-ignore lint/suspicious/noArrayIndexKey: diff lines are positional
            key={i}
            className={cn(
              'flex gap-3 px-3',
              l.kind === 'add' && 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300',
              l.kind === 'del' && 'bg-red-500/15 text-red-700 dark:text-red-300',
            )}
          >
            <span className="w-4 shrink-0 select-none text-muted-foreground">
              {l.kind === 'add' ? '+' : l.kind === 'del' ? '-' : ' '}
            </span>
            <span className="whitespace-pre-wrap break-all">{l.text}</span>
          </div>
        ))}
      </pre>
    </div>
  )
}

export function SubjectPage() {
  const { clusterId, subject } = useParams({ from: '/clusters/$clusterId/schemas/$subject' })
  const navigate = useNavigate()
  const { data, isPending, error } = useSubject(clusterId, subject)
  const canMutate = useCanMutate(clusterId)
  const setCompat = useSetCompatibility(clusterId, subject)
  const del = useDeleteSubject(clusterId, subject)
  const [selected, setSelected] = useState<number | null>(null)
  const [compare, setCompare] = useState<number | null>(null)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [permanent, setPermanent] = useState(false)

  const latest = data?.versions[0]
  const current = data?.versions.find((v) => v.version === selected) ?? latest
  const other = data?.versions.find((v) => v.version === compare)

  return (
    <>
      <div>
        <Link
          to="/clusters/$clusterId/schemas"
          params={{ clusterId }}
          className="inline-flex items-center gap-1 text-muted-foreground text-sm hover:text-foreground"
        >
          <ChevronLeftIcon className="size-4" /> Schema Registry
        </Link>
      </div>
      <PageHeader
        title={<span className="font-mono">{subject}</span>}
        description={
          latest && (
            <span className="inline-flex items-center gap-2">
              <SchemaTypeBadge type={latest.schemaType} /> {data.versions.length} versions · latest v{latest.version}{' '}
              (id {latest.id})
            </span>
          )
        }
        actions={
          canMutate &&
          latest && (
            <>
              <RegisterSchemaDialog
                clusterId={clusterId}
                subject={subject}
                initialSchema={formatSchema(latest)}
                initialType={latest.schemaType}
                onRegistered={() => setSelected(null)}
              />
              <Button size="sm" variant="outline" onClick={() => setConfirmDelete(true)}>
                <Trash2Icon /> Delete
              </Button>
            </>
          )
        }
      />
      {error ? (
        <ErrorState error={error} />
      ) : isPending ? (
        <LoadingRows rows={8} />
      ) : (
        current && (
          <div className="grid gap-4 lg:grid-cols-[1fr_18rem]">
            <Card>
              <CardHeader className="flex flex-row flex-wrap items-center gap-3 space-y-0">
                <CardTitle className="mr-auto">Schema</CardTitle>
                <Select value={String(current.version)} onValueChange={(v) => setSelected(Number(v))}>
                  <SelectTrigger aria-label="Version" className="w-40">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {data.versions.map((v) => (
                      <SelectItem key={v.version} value={String(v.version)}>
                        v{v.version} · id {v.id}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Select
                  value={compare === null ? 'none' : String(compare)}
                  onValueChange={(v) => setCompare(v === 'none' ? null : Number(v))}
                  disabled={data.versions.length < 2}
                >
                  <SelectTrigger aria-label="Compare with" className="w-44">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">No comparison</SelectItem>
                    {data.versions
                      .filter((v) => v.version !== current.version)
                      .map((v) => (
                        <SelectItem key={v.version} value={String(v.version)}>
                          Compare with v{v.version}
                        </SelectItem>
                      ))}
                  </SelectContent>
                </Select>
                <CopyButton text={current.schema} />
              </CardHeader>
              <CardContent>
                {other && other.version !== current.version ? (
                  <SchemaDiff
                    from={other.version < current.version ? other : current}
                    to={other.version < current.version ? current : other}
                  />
                ) : (
                  <pre className="max-h-[60vh] overflow-auto whitespace-pre-wrap break-all rounded-md border bg-muted/40 p-3 font-mono text-xs">
                    {formatSchema(current)}
                  </pre>
                )}
              </CardContent>
            </Card>
            <div className="flex flex-col gap-4">
              <Card>
                <CardHeader>
                  <CardTitle>Compatibility</CardTitle>
                </CardHeader>
                <CardContent className="flex flex-col gap-2 text-sm">
                  {canMutate ? (
                    <Select
                      value={data.compatibility || inherit}
                      disabled={setCompat.isPending}
                      onValueChange={(v) =>
                        setCompat.mutate(v === inherit ? '' : v, {
                          onSuccess: () => toast.success('Compatibility updated'),
                          onError: (e) => toast.error(e.message),
                        })
                      }
                    >
                      <SelectTrigger aria-label="Compatibility level" className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value={inherit}>Inherit global ({data.globalCompatibility})</SelectItem>
                        {compatibilityLevels.map((l) => (
                          <SelectItem key={l} value={l}>
                            {l}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  ) : (
                    <span className="font-mono">{data.compatibility || `${data.globalCompatibility} (global)`}</span>
                  )}
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardTitle>References</CardTitle>
                </CardHeader>
                <CardContent className="text-sm">
                  {current.references.length === 0 ? (
                    <span className="text-muted-foreground">None</span>
                  ) : (
                    <ul className="flex flex-col gap-1">
                      {current.references.map((r) => (
                        <li key={r.name}>
                          <Link
                            to="/clusters/$clusterId/schemas/$subject"
                            params={{ clusterId, subject: r.subject }}
                            className="font-mono text-xs hover:underline"
                          >
                            {r.name}
                          </Link>{' '}
                          <Badge variant="outline">
                            {r.subject} v{r.version}
                          </Badge>
                        </li>
                      ))}
                    </ul>
                  )}
                </CardContent>
              </Card>
            </div>
          </div>
        )
      )}
      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={(o) => {
          setConfirmDelete(o)
          if (!o) {
            setPermanent(false)
            del.reset()
          }
        }}
        title={`Delete ${subject}?`}
        description={
          <div className="flex flex-col gap-3">
            <span>
              A soft delete hides the subject but keeps schema IDs readable, so existing messages still decode. A
              permanent delete removes them for good.
            </span>
            <span className="flex items-center gap-2">
              <Checkbox id="permanent" checked={permanent} onCheckedChange={(c) => setPermanent(c === true)} />
              <Label htmlFor="permanent" className="font-normal">
                Delete permanently
              </Label>
            </span>
          </div>
        }
        confirmLabel={permanent ? 'Delete permanently' : 'Soft delete'}
        confirmText={subject}
        pending={del.isPending}
        error={del.error}
        onConfirm={() =>
          del.mutate(permanent, {
            onSuccess: () => {
              toast.success(`Deleted ${subject}`)
              navigate({ to: '/clusters/$clusterId/schemas', params: { clusterId } })
            },
          })
        }
      />
    </>
  )
}
