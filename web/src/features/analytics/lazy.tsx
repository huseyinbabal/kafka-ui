// Charts pull in Recharts (~300 KB); load them only where a chart is shown.
import { type ComponentProps, lazy, Suspense } from 'react'
import { LoadingRows } from '@/components/states'

const LazyTopicThroughput = lazy(() => import('./series-cards').then((m) => ({ default: m.TopicThroughput })))
const LazyGroupLagTrend = lazy(() => import('./series-cards').then((m) => ({ default: m.GroupLagTrend })))

type TopicProps = ComponentProps<typeof LazyTopicThroughput>
type GroupProps = ComponentProps<typeof LazyGroupLagTrend>

export function TopicThroughput(props: TopicProps) {
  return (
    <Suspense fallback={<LoadingRows rows={4} />}>
      <LazyTopicThroughput {...props} />
    </Suspense>
  )
}

export function GroupLagTrend(props: GroupProps) {
  return (
    <Suspense fallback={<LoadingRows rows={3} />}>
      <LazyGroupLagTrend {...props} />
    </Suspense>
  )
}
