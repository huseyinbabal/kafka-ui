import { useParams } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { PlusIcon, ShieldOffIcon, Trash2Icon } from 'lucide-react'
import { type FormEvent, useMemo, useState } from 'react'
import { toast } from 'sonner'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { DataTable } from '@/components/data-table'
import { PageHeader } from '@/components/page'
import { EmptyState, ErrorState } from '@/components/states'
import { Badge } from '@/components/ui/badge'
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
import { useCanMutate } from '@/features/auth/api'
import { ApiError } from '@/lib/api'
import { type ACL, operations, resourceTypes, useACLs, useCreateACL, useDeleteACL } from './api'

function Field({ id, label, children }: { id: string; label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-2">
      <Label htmlFor={id}>{label}</Label>
      {children}
    </div>
  )
}

function Choice({
  id,
  value,
  onChange,
  options,
}: {
  id: string
  value: string
  onChange: (v: string) => void
  options: readonly string[]
}) {
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger id={id} className="w-full">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((o) => (
          <SelectItem key={o} value={o}>
            {o}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

const blank: ACL = {
  principal: 'User:',
  host: '*',
  resourceType: 'TOPIC',
  resourceName: '',
  patternType: 'LITERAL',
  operation: 'READ',
  permission: 'ALLOW',
}

function CreateACLDialog({ clusterId }: { clusterId: string }) {
  const [open, setOpen] = useState(false)
  const [acl, setAcl] = useState<ACL>(blank)
  const create = useCreateACL(clusterId)
  const set = (k: keyof ACL) => (v: string) => setAcl((a) => ({ ...a, [k]: v }))
  const isCluster = acl.resourceType === 'CLUSTER'

  const submit = (e: FormEvent) => {
    e.preventDefault()
    create.mutate(isCluster ? { ...acl, resourceName: 'kafka-cluster' } : acl, {
      onSuccess: () => {
        toast.success('ACL created')
        setOpen(false)
        setAcl(blank)
      },
    })
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        setOpen(o)
        if (!o) create.reset()
      }}
    >
      <DialogTrigger asChild>
        <Button size="sm">
          <PlusIcon /> Add ACL
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <form onSubmit={submit} className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle>Add ACL</DialogTitle>
            <DialogDescription>Grants or denies one operation on one resource to one principal.</DialogDescription>
          </DialogHeader>
          <div className="grid grid-cols-2 gap-4">
            <Field id="acl-principal" label="Principal">
              <Input
                id="acl-principal"
                className="font-mono"
                value={acl.principal}
                onChange={(e) => set('principal')(e.target.value)}
              />
            </Field>
            <Field id="acl-host" label="Host">
              <Input
                id="acl-host"
                className="font-mono"
                value={acl.host}
                onChange={(e) => set('host')(e.target.value)}
              />
            </Field>
            <Field id="acl-type" label="Resource type">
              <Choice id="acl-type" value={acl.resourceType} onChange={set('resourceType')} options={resourceTypes} />
            </Field>
            <Field id="acl-pattern" label="Pattern">
              <Choice
                id="acl-pattern"
                value={acl.patternType}
                onChange={set('patternType')}
                options={['LITERAL', 'PREFIXED']}
              />
            </Field>
            {!isCluster && (
              <div className="col-span-2">
                <Field
                  id="acl-name"
                  label={acl.patternType === 'PREFIXED' ? 'Resource name prefix' : 'Resource name (* for all)'}
                >
                  <Input
                    id="acl-name"
                    className="font-mono"
                    value={acl.resourceName}
                    onChange={(e) => set('resourceName')(e.target.value)}
                  />
                </Field>
              </div>
            )}
            <Field id="acl-op" label="Operation">
              <Choice id="acl-op" value={acl.operation} onChange={set('operation')} options={operations} />
            </Field>
            <Field id="acl-perm" label="Permission">
              <Choice id="acl-perm" value={acl.permission} onChange={set('permission')} options={['ALLOW', 'DENY']} />
            </Field>
          </div>
          {create.error && <p className="text-destructive text-sm">{create.error.message}</p>}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={create.isPending || (!isCluster && !acl.resourceName)}>
              {create.isPending ? 'Saving…' : 'Add ACL'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export function ACLsPage() {
  const { clusterId } = useParams({ from: '/clusters/$clusterId/acls' })
  const { data = [], isPending, error } = useACLs(clusterId)
  const canMutate = useCanMutate(clusterId)
  const del = useDeleteACL(clusterId)
  const [deleting, setDeleting] = useState<ACL | null>(null)
  const disabled = error instanceof ApiError && error.code === 'SECURITY_DISABLED'

  const columns = useMemo<ColumnDef<ACL>[]>(() => {
    const cols: ColumnDef<ACL>[] = [
      {
        accessorKey: 'principal',
        header: 'Principal',
        cell: ({ getValue }) => <span className="font-mono text-xs">{getValue<string>()}</span>,
      },
      {
        accessorKey: 'permission',
        header: 'Permission',
        cell: ({ getValue }) => (
          <Badge variant={getValue<string>() === 'DENY' ? 'destructive' : 'secondary'}>{getValue<string>()}</Badge>
        ),
      },
      { accessorKey: 'operation', header: 'Operation' },
      { accessorKey: 'resourceType', header: 'Resource' },
      {
        accessorKey: 'resourceName',
        header: 'Name',
        cell: ({ row }) => (
          <span className="font-mono text-xs">
            {row.original.resourceName}
            {row.original.patternType === 'PREFIXED' && <span className="text-muted-foreground">*</span>}
          </span>
        ),
      },
      {
        accessorKey: 'patternType',
        header: 'Pattern',
        cell: ({ getValue }) => <span className="text-muted-foreground text-xs">{getValue<string>()}</span>,
      },
      {
        accessorKey: 'host',
        header: 'Host',
        cell: ({ getValue }) => <span className="font-mono text-xs">{getValue<string>()}</span>,
      },
    ]
    if (canMutate) {
      cols.push({
        id: 'delete',
        header: '',
        enableSorting: false,
        meta: { align: 'right' },
        cell: ({ row }) => (
          <Button variant="ghost" size="icon-sm" aria-label="Delete ACL" onClick={() => setDeleting(row.original)}>
            <Trash2Icon />
          </Button>
        ),
      })
    }
    return cols
  }, [canMutate])

  return (
    <>
      <PageHeader
        title="ACLs"
        description={isPending || disabled ? undefined : `${data.length} entries`}
        actions={canMutate && !disabled && <CreateACLDialog clusterId={clusterId} />}
      />
      {disabled ? (
        <EmptyState
          icon={<ShieldOffIcon className="size-8 opacity-50" />}
          title="No authorizer on this cluster"
          description="The brokers run without authorizer.class.name, so every client may do everything and ACLs cannot be listed or created."
        />
      ) : error ? (
        <ErrorState error={error} />
      ) : (
        <DataTable
          columns={columns}
          data={data}
          loading={isPending}
          searchPlaceholder="Search ACLs…"
          emptyState={
            <EmptyState title="No ACLs" description="With an authorizer and no ACLs, only super users have access." />
          }
        />
      )}
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => {
          if (!o) {
            setDeleting(null)
            del.reset()
          }
        }}
        title="Delete ACL?"
        description={
          deleting && (
            <span className="font-mono text-xs">
              {deleting.permission} {deleting.principal} {deleting.operation} on {deleting.resourceType}{' '}
              {deleting.resourceName} ({deleting.patternType}) from {deleting.host}
            </span>
          )
        }
        confirmLabel="Delete ACL"
        pending={del.isPending}
        error={del.error}
        onConfirm={() =>
          deleting &&
          del.mutate(deleting, {
            onSuccess: () => {
              toast.success('ACL deleted')
              setDeleting(null)
            },
          })
        }
      />
    </>
  )
}
