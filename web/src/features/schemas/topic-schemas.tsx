import { Link } from '@tanstack/react-router'
import { EmptyState, ErrorState, LoadingRows } from '@/components/states'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatSchema, useSubject, useSubjects } from './api'
import { SchemaTypeBadge } from './schemas-page'

function SubjectCard({ clusterId, subject, role }: { clusterId: string; subject: string; role: string }) {
  const { data, error, isPending } = useSubject(clusterId, subject)
  const latest = data?.versions[0]
  return (
    <Card>
      <CardHeader className="flex flex-row items-center gap-2 space-y-0">
        <CardTitle className="mr-auto">
          {role}:{' '}
          <Link
            to="/clusters/$clusterId/schemas/$subject"
            params={{ clusterId, subject }}
            className="font-mono hover:underline"
          >
            {subject}
          </Link>
        </CardTitle>
        {latest && (
          <>
            <SchemaTypeBadge type={latest.schemaType} />
            <span className="text-muted-foreground text-xs">
              v{latest.version} · id {latest.id}
            </span>
          </>
        )}
      </CardHeader>
      <CardContent>
        {error ? (
          <ErrorState error={error} />
        ) : isPending || !latest ? (
          <LoadingRows rows={3} />
        ) : (
          <pre className="max-h-80 overflow-auto whitespace-pre-wrap break-all rounded-md border bg-muted/40 p-3 font-mono text-xs">
            {formatSchema(latest)}
          </pre>
        )}
      </CardContent>
    </Card>
  )
}

/** Shows the TopicNameStrategy subjects (<topic>-key / <topic>-value) of a topic. */
export function TopicSchemas({ clusterId, topic }: { clusterId: string; topic: string }) {
  const { data, error, isPending } = useSubjects(clusterId)
  if (error) return <ErrorState error={error} />
  if (isPending) return <LoadingRows rows={4} />
  const found = [
    { role: 'Key', subject: `${topic}-key` },
    { role: 'Value', subject: `${topic}-value` },
  ].filter((s) => data.some((d) => d.subject === s.subject))
  if (found.length === 0) {
    return (
      <EmptyState
        title="No schema for this topic"
        description={`No ${topic}-key or ${topic}-value subject exists (TopicNameStrategy). Messages using other subject strategies still decode by schema id.`}
      />
    )
  }
  return (
    <div className="grid gap-4">
      {found.map((s) => (
        <SubjectCard key={s.subject} clusterId={clusterId} {...s} />
      ))}
    </div>
  )
}
