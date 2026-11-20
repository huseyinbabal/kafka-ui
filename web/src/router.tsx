import type { QueryClient } from '@tanstack/react-query'
import {
  createRootRouteWithContext,
  createRoute,
  createRouter,
  lazyRouteComponent,
  Outlet,
  redirect,
} from '@tanstack/react-router'
import { ClusterLayout } from '@/components/app-layout'
import { EmptyState, ErrorState } from '@/components/states'
import { ACLsPage } from '@/features/acls/acls-page'
import { AnalyticsPage } from '@/features/analytics/analytics-page'
import { meKey } from '@/features/auth/api'
import { LoginPage } from '@/features/auth/login-page'
import { BrokersPage } from '@/features/clusters/brokers-page'
import { OverviewPage } from '@/features/clusters/overview-page'
import { GroupPage } from '@/features/groups/group-page'
import { GroupsPage } from '@/features/groups/groups-page'
import { SchemasPage } from '@/features/schemas/schemas-page'
import { SubjectPage } from '@/features/schemas/subject-page'
import { TopicPage } from '@/features/topics/topic-page'
import { TopicsPage } from '@/features/topics/topics-page'
import { ApiError, type ClusterInfo, get, type List, type Principal } from '@/lib/api'

type Ctx = { queryClient: QueryClient }

async function loadMe(qc: QueryClient): Promise<Principal | null> {
  return qc.ensureQueryData({
    queryKey: meKey,
    queryFn: async () => {
      try {
        return await get<Principal>('/auth/me')
      } catch (e) {
        if (e instanceof ApiError && e.status === 401) return null
        throw e
      }
    },
  })
}

async function requireAuth({ context, location }: { context: Ctx; location: { href: string } }) {
  const me = await loadMe(context.queryClient)
  if (!me) throw redirect({ to: '/login', search: { next: location.href } })
}

const rootRoute = createRootRouteWithContext<Ctx>()({
  component: Outlet,
  notFoundComponent: () => <EmptyState title="Page not found" />,
  errorComponent: ({ error }) => (
    <div className="p-6">
      <ErrorState error={error} />
    </div>
  ),
})

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  validateSearch: (s: Record<string, unknown>): { next?: string } =>
    typeof s.next === 'string' && s.next.startsWith('/') && !s.next.startsWith('//') ? { next: s.next } : {},
  beforeLoad: async ({ context }) => {
    if (await loadMe(context.queryClient)) throw redirect({ to: '/' })
  },
  component: LoginPage,
})

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  beforeLoad: async (opts) => {
    await requireAuth(opts)
    const { items } = await opts.context.queryClient.fetchQuery({
      queryKey: ['clusters', 'raw'],
      queryFn: () => get<List<ClusterInfo>>('/clusters'),
    })
    if (items.length > 0) throw redirect({ to: '/clusters/$clusterId', params: { clusterId: items[0].id } })
  },
  component: () => <EmptyState title="No clusters configured" description="Add clusters to config.yaml." />,
})

const clusterRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/clusters/$clusterId',
  beforeLoad: requireAuth,
  component: ClusterLayout,
})

const overviewRoute = createRoute({ getParentRoute: () => clusterRoute, path: '/', component: OverviewPage })
const brokersRoute = createRoute({ getParentRoute: () => clusterRoute, path: 'brokers', component: BrokersPage })

const topicsRoute = createRoute({ getParentRoute: () => clusterRoute, path: 'topics' })
const topicsIndexRoute = createRoute({ getParentRoute: () => topicsRoute, path: '/', component: TopicsPage })
const topicRoute = createRoute({ getParentRoute: () => topicsRoute, path: '$topic', component: TopicPage })

const groupsRoute = createRoute({ getParentRoute: () => clusterRoute, path: 'groups' })
const groupsIndexRoute = createRoute({ getParentRoute: () => groupsRoute, path: '/', component: GroupsPage })
const groupRoute = createRoute({ getParentRoute: () => groupsRoute, path: '$group', component: GroupPage })

const schemasRoute = createRoute({ getParentRoute: () => clusterRoute, path: 'schemas' })
const schemasIndexRoute = createRoute({ getParentRoute: () => schemasRoute, path: '/', component: SchemasPage })
const subjectRoute = createRoute({ getParentRoute: () => schemasRoute, path: '$subject', component: SubjectPage })
const analyticsRoute = createRoute({ getParentRoute: () => clusterRoute, path: 'analytics', component: AnalyticsPage })
// three.js is large: load the live view only when it is opened.
const liveRoute = createRoute({
  getParentRoute: () => clusterRoute,
  path: 'live',
  component: lazyRouteComponent(() => import('@/features/live/live-page'), 'LivePage'),
})
const aclsRoute = createRoute({ getParentRoute: () => clusterRoute, path: 'acls', component: ACLsPage })

const routeTree = rootRoute.addChildren([
  loginRoute,
  indexRoute,
  clusterRoute.addChildren([
    overviewRoute,
    brokersRoute,
    topicsRoute.addChildren([topicsIndexRoute, topicRoute]),
    groupsRoute.addChildren([groupsIndexRoute, groupRoute]),
    schemasRoute.addChildren([schemasIndexRoute, subjectRoute]),
    aclsRoute,
    analyticsRoute,
    liveRoute,
  ]),
])

export function makeRouter(queryClient: QueryClient) {
  return createRouter({ routeTree, context: { queryClient }, defaultPreload: 'intent' })
}

declare module '@tanstack/react-router' {
  interface Register {
    router: ReturnType<typeof makeRouter>
  }
}
