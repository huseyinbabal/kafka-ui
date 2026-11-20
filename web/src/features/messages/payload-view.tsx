import { CheckIcon, CopyIcon } from 'lucide-react'
import { useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { prettyJson } from '@/lib/json'
import type { Payload } from './api'

export function payloadPreview(p: Payload | null, max = 160): string {
  if (!p) return 'null'
  const t = p.text.replace(/\s+/g, ' ')
  return t.length > max ? `${t.slice(0, max)}…` : t
}

export function CopyButton({ text, label = 'Copy' }: { text: string; label?: string }) {
  const [done, setDone] = useState(false)
  return (
    <Button
      variant="ghost"
      size="sm"
      onClick={() => {
        void navigator.clipboard.writeText(text).then(() => {
          setDone(true)
          setTimeout(() => setDone(false), 1200)
        })
      }}
    >
      {done ? <CheckIcon /> : <CopyIcon />}
      {label}
    </Button>
  )
}

export function PayloadView({ title, payload }: { title: string; payload: Payload | null }) {
  const text = payload ? (payload.format === 'json' ? prettyJson(payload.text) : payload.text) : ''
  return (
    <section className="flex flex-col gap-2">
      <div className="flex items-center gap-2">
        <h3 className="font-medium text-sm">{title}</h3>
        {payload ? (
          <>
            <Badge variant="secondary">{payload.format}</Badge>
            <span className="text-muted-foreground text-xs">{payload.size} B</span>
            {payload.schemaId !== undefined && <Badge variant="outline">schema id {payload.schemaId}</Badge>}
            {payload.truncated && <Badge variant="outline">truncated</Badge>}
          </>
        ) : (
          <Badge variant="outline">null</Badge>
        )}
        <div className="ml-auto">{payload && <CopyButton text={payload.text} />}</div>
      </div>
      {payload && (
        <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-all rounded-md border bg-muted/40 p-3 font-mono text-xs">
          {text || <span className="text-muted-foreground">(empty)</span>}
        </pre>
      )}
    </section>
  )
}
