import { Link, Outlet, useNavigate, useParams, useRouterState } from '@tanstack/react-router'
import {
  ActivityIcon,
  ChartLineIcon,
  CheckIcon,
  ChevronsUpDownIcon,
  FileJson2Icon,
  LayoutDashboardIcon,
  ListTreeIcon,
  LogOutIcon,
  MoonIcon,
  ServerIcon,
  ShieldIcon,
  SunIcon,
  UsersIcon,
  WorkflowIcon,
} from 'lucide-react'
import { useTheme } from 'next-themes'
import type { ComponentType } from 'react'
import { StatusDot } from '@/components/page'
import { Badge } from '@/components/ui/badge'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Separator } from '@/components/ui/separator'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarTrigger,
} from '@/components/ui/sidebar'
import { useLogout, useMe } from '@/features/auth/api'
import { useClusters } from '@/features/clusters/api'

type NavItem = {
  to: string
  label: string
  icon: ComponentType<{ className?: string }>
  exact?: boolean
  soon?: boolean
  needsSchemaRegistry?: boolean
}

const nav: { label: string; items: NavItem[] }[] = [
  {
    label: 'Cluster',
    items: [
      { to: '/clusters/$clusterId', label: 'Overview', icon: LayoutDashboardIcon, exact: true },
      { to: '/clusters/$clusterId/brokers', label: 'Brokers', icon: ServerIcon },
      { to: '/clusters/$clusterId/topics', label: 'Topics', icon: ListTreeIcon },
      { to: '/clusters/$clusterId/groups', label: 'Consumer groups', icon: UsersIcon },
    ],
  },
  {
    label: 'Governance',
    items: [
      { to: '/clusters/$clusterId/schemas', label: 'Schema Registry', icon: FileJson2Icon, needsSchemaRegistry: true },
      { to: '/clusters/$clusterId/acls', label: 'ACLs', icon: ShieldIcon },
    ],
  },
  {
    label: 'Insights',
    items: [
      { to: '/clusters/$clusterId/analytics', label: 'Analytics', icon: ChartLineIcon },
      { to: '/clusters/$clusterId/live', label: 'Live topology', icon: ActivityIcon, soon: true },
    ],
  },
]

function ClusterSwitcher({ clusterId }: { clusterId: string }) {
  const { data: clusters = [] } = useClusters()
  const navigate = useNavigate()
  const current = clusters.find((c) => c.id === clusterId)
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <SidebarMenuButton size="lg" className="data-[state=open]:bg-sidebar-accent">
          <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
            <WorkflowIcon className="size-4" />
          </div>
          <div className="grid flex-1 text-left text-sm leading-tight">
            <span className="truncate font-semibold">{current?.name ?? clusterId}</span>
            <span className="flex items-center gap-1.5 truncate text-muted-foreground text-xs">
              <StatusDot status={current?.status ?? 'unknown'} />
              {current?.status ?? 'unknown'}
              {current?.readOnly && ' · read-only'}
            </span>
          </div>
          <ChevronsUpDownIcon className="ml-auto size-4" />
        </SidebarMenuButton>
      </DropdownMenuTrigger>
      <DropdownMenuContent className="w-(--radix-dropdown-menu-trigger-width) min-w-56" align="start">
        <DropdownMenuLabel className="text-muted-foreground text-xs">Clusters</DropdownMenuLabel>
        {clusters.map((c) => (
          <DropdownMenuItem
            key={c.id}
            onSelect={() => navigate({ to: '/clusters/$clusterId', params: { clusterId: c.id } })}
          >
            <StatusDot status={c.status} />
            <span className="flex-1 truncate">{c.name}</span>
            {c.id === clusterId && <CheckIcon className="size-4" />}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function UserMenu() {
  const { data: me } = useMe()
  const logout = useLogout()
  const navigate = useNavigate()
  const { resolvedTheme, setTheme } = useTheme()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <SidebarMenuButton size="lg">
          <div className="flex size-8 items-center justify-center rounded-full bg-muted font-medium text-xs uppercase">
            {me?.username.slice(0, 2)}
          </div>
          <div className="grid flex-1 text-left text-sm leading-tight">
            <span className="truncate font-medium">{me?.username}</span>
            <span className="truncate text-muted-foreground text-xs">{me?.role}</span>
          </div>
          <ChevronsUpDownIcon className="ml-auto size-4" />
        </SidebarMenuButton>
      </DropdownMenuTrigger>
      <DropdownMenuContent side="top" align="start" className="w-(--radix-dropdown-menu-trigger-width) min-w-48">
        <DropdownMenuItem onSelect={() => setTheme(resolvedTheme === 'dark' ? 'light' : 'dark')}>
          {resolvedTheme === 'dark' ? <SunIcon /> : <MoonIcon />}
          {resolvedTheme === 'dark' ? 'Light mode' : 'Dark mode'}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={() => logout.mutate(undefined, { onSuccess: () => navigate({ to: '/login' }) })}>
          <LogOutIcon />
          Sign out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function ClusterLayout() {
  const { clusterId } = useParams({ strict: false }) as { clusterId: string }
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const base = `/clusters/${clusterId}`
  const { data: clusters = [] } = useClusters()
  const hasSchemaRegistry = clusters.find((c) => c.id === clusterId)?.hasSchemaRegistry ?? false

  return (
    <SidebarProvider>
      <Sidebar collapsible="icon">
        <SidebarHeader>
          <SidebarMenu>
            <SidebarMenuItem>
              <ClusterSwitcher clusterId={clusterId} />
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarHeader>
        <SidebarContent>
          {nav.map((group) => (
            <SidebarGroup key={group.label}>
              <SidebarGroupLabel>{group.label}</SidebarGroupLabel>
              <SidebarMenu>
                {group.items.map((item) => {
                  const href = item.to.replace('/clusters/$clusterId', base)
                  const active = item.exact ? pathname === href : pathname.startsWith(href)
                  const off = item.soon ? 'soon' : item.needsSchemaRegistry && !hasSchemaRegistry ? 'none' : null
                  return (
                    <SidebarMenuItem key={item.to}>
                      <SidebarMenuButton
                        asChild
                        isActive={active}
                        tooltip={off === 'none' ? `${item.label}: not configured for this cluster` : item.label}
                        disabled={!!off}
                      >
                        {off ? (
                          <span className="opacity-50">
                            <item.icon />
                            <span>{item.label}</span>
                            <Badge variant="outline" className="ml-auto text-[10px]">
                              {off}
                            </Badge>
                          </span>
                        ) : (
                          <Link to={item.to} params={{ clusterId }}>
                            <item.icon />
                            <span>{item.label}</span>
                          </Link>
                        )}
                      </SidebarMenuButton>
                    </SidebarMenuItem>
                  )
                })}
              </SidebarMenu>
            </SidebarGroup>
          ))}
        </SidebarContent>
        <SidebarFooter>
          <SidebarMenu>
            <SidebarMenuItem>
              <UserMenu />
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
      </Sidebar>
      <SidebarInset>
        <header className="sticky top-0 z-10 flex h-12 shrink-0 items-center gap-2 border-b bg-background/80 px-4 backdrop-blur">
          <SidebarTrigger className="-ml-1" />
          <Separator orientation="vertical" className="mr-2 h-4" />
          <span className="text-muted-foreground text-sm">{clusterId}</span>
        </header>
        <main className="mx-auto grid w-full max-w-7xl gap-6 p-4 md:p-6">
          <Outlet />
        </main>
      </SidebarInset>
    </SidebarProvider>
  )
}
