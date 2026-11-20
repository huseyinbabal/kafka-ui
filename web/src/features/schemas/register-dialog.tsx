import { CheckCircle2Icon, PlusIcon, XCircleIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { type SchemaType, useCheckCompatibility, useRegisterSchema } from './api'

type Props = {
  clusterId: string
  /** Fixed subject (new version) or undefined (new subject: the user types it). */
  subject?: string
  initialSchema?: string
  initialType?: SchemaType
  onRegistered?: (subject: string) => void
  trigger?: React.ReactNode
}

export function RegisterSchemaDialog({
  clusterId,
  subject,
  initialSchema = '',
  initialType = 'AVRO',
  onRegistered,
  trigger,
}: Props) {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState(subject ?? '')
  const [schemaType, setSchemaType] = useState<SchemaType>(initialType)
  const [schema, setSchema] = useState(initialSchema)
  const target = subject ?? name
  const check = useCheckCompatibility(clusterId, target)
  const register = useRegisterSchema(clusterId, target)
  const input = { schema, schemaType }

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        setOpen(o)
        if (o) {
          setSchema(initialSchema)
          setSchemaType(initialType)
        } else {
          check.reset()
          register.reset()
        }
      }}
    >
      <DialogTrigger asChild>
        {trigger ?? (
          <Button size="sm">
            <PlusIcon /> {subject ? 'New version' : 'New subject'}
          </Button>
        )}
      </DialogTrigger>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{subject ? `New version of ${subject}` : 'Register a schema'}</DialogTitle>
          <DialogDescription>
            The registry checks compatibility on register. Registering an identical schema returns the existing version.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 sm:grid-cols-[1fr_auto]">
          {!subject && (
            <div className="flex flex-col gap-2">
              <Label htmlFor="schema-subject">Subject</Label>
              <Input
                id="schema-subject"
                className="font-mono"
                placeholder="orders-value"
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </div>
          )}
          <div className="flex flex-col gap-2">
            <Label htmlFor="schema-type">Type</Label>
            <Select value={schemaType} onValueChange={(v) => setSchemaType(v as SchemaType)} disabled={!!subject}>
              <SelectTrigger id="schema-type" className="w-36">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="AVRO">Avro</SelectItem>
                <SelectItem value="PROTOBUF">Protobuf</SelectItem>
                <SelectItem value="JSON">JSON Schema</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="schema-text">Schema</Label>
          <Textarea
            id="schema-text"
            rows={16}
            className="max-h-[50vh] font-mono text-xs"
            value={schema}
            onChange={(e) => {
              setSchema(e.target.value)
              check.reset()
            }}
          />
        </div>
        {check.data &&
          (check.data.compatible ? (
            <Alert>
              <CheckCircle2Icon />
              <AlertTitle>Compatible with the latest version</AlertTitle>
            </Alert>
          ) : (
            <Alert variant="destructive">
              <XCircleIcon />
              <AlertTitle>Not compatible</AlertTitle>
              <AlertDescription>
                <ul className="list-disc pl-4">
                  {check.data.messages.map((m) => (
                    <li key={m} className="break-all font-mono text-xs">
                      {m}
                    </li>
                  ))}
                </ul>
              </AlertDescription>
            </Alert>
          ))}
        {(check.error || register.error) && (
          <p className="text-destructive text-sm">{(register.error ?? check.error)?.message}</p>
        )}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => setOpen(false)}>
            Cancel
          </Button>
          <Button
            type="button"
            variant="secondary"
            disabled={!target || !schema.trim() || check.isPending}
            onClick={() => check.mutate(input)}
          >
            Check compatibility
          </Button>
          <Button
            type="button"
            disabled={!target || !schema.trim() || register.isPending}
            onClick={() =>
              register.mutate(input, {
                onSuccess: (v) => {
                  toast.success(`Registered ${v.subject} version ${v.version} (id ${v.id})`)
                  setOpen(false)
                  onRegistered?.(v.subject)
                },
              })
            }
          >
            {register.isPending ? 'Registering…' : 'Register'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
