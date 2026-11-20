import { useNavigate, useSearch } from '@tanstack/react-router'
import { Loader2Icon, WorkflowIcon } from 'lucide-react'
import { type FormEvent, useState } from 'react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useLogin } from './api'

export function LoginPage() {
  const login = useLogin()
  const navigate = useNavigate()
  const { next } = useSearch({ from: '/login' })
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')

  function onSubmit(e: FormEvent) {
    e.preventDefault()
    login.mutate({ username, password }, { onSuccess: () => (next ? navigate({ href: next }) : navigate({ to: '/' })) })
  }

  return (
    <div className="flex min-h-svh items-center justify-center bg-muted/40 p-4">
      <Card className="w-full max-w-sm">
        <CardHeader className="items-center text-center">
          <div className="mx-auto mb-2 flex size-10 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <WorkflowIcon className="size-5" />
          </div>
          <CardTitle className="text-xl">Kafka UI</CardTitle>
          <CardDescription>Sign in to continue</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="grid gap-4" onSubmit={onSubmit}>
            {login.error && (
              <Alert variant="destructive">
                <AlertDescription>{login.error.message}</AlertDescription>
              </Alert>
            )}
            <div className="grid gap-2">
              <Label htmlFor="username">Username</Label>
              <Input
                id="username"
                autoComplete="username"
                autoFocus
                required
                value={username}
                onChange={(e) => setUsername(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                type="password"
                autoComplete="current-password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>
            <Button type="submit" disabled={login.isPending}>
              {login.isPending && <Loader2Icon className="animate-spin" />}
              Sign in
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
