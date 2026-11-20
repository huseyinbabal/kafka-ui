// Pure graph logic for the live view: merging snapshots without re-layout,
// filtering, and node status. Kept free of React and three.js for testing.

export type NodeKind = 'broker' | 'topic' | 'group' | 'member'
export type LinkKind = 'leader' | 'member' | 'consumes' | 'offsets'

export type SnapshotNode = {
  id: string
  kind: NodeKind
  label: string
  rate: number
  controller?: boolean
  host?: string
  leaders?: number
  partitions?: number
  internal?: boolean
  state?: string
  lag?: number
  members?: number
  group?: string
  clientId?: string
}
export type SnapshotLink = { source: string; target: string; kind: LinkKind; partitions: number; rate: number }
export type Snapshot = { at: number; nodes: SnapshotNode[]; links: SnapshotLink[] }

/** Nodes carry simulation state (x, y, z, vx…) that must survive updates. */
export type GraphNode = SnapshotNode & {
  bornAt: number
  x?: number
  y?: number
  z?: number
  [sim: string]: unknown
}
/** Endpoints are ids; kept as a union so graph code also accepts resolved node objects. */
export type GraphLink = Omit<SnapshotLink, 'source' | 'target'> & {
  key: string
  source: string | GraphNode
  target: string | GraphNode
}
export type Graph = { nodes: GraphNode[]; links: GraphLink[] }
export type Change = { kind: 'joined' | 'left'; node: SnapshotNode; at: number }

export const emptyGraph: Graph = { nodes: [], links: [] }
export const linkKey = (l: { source: string; target: string; kind: string }) => `${l.kind}:${l.source}->${l.target}`
export const endpointId = (e: string | GraphNode) => (typeof e === 'string' ? e : e.id)

/**
 * Applies a snapshot to the previous graph. Existing nodes and links keep their
 * object identity (and so their positions); new members start next to their
 * group so they don't fly in from the origin. `structural` says whether the
 * node or link set changed (only then does the layout need new data).
 */
export function mergeSnapshot(prev: Graph, snap: Snapshot, now: number) {
  const prevNodes = new Map(prev.nodes.map((n) => [n.id, n]))
  const nodes: GraphNode[] = []
  const changes: Change[] = []
  let structural = prev.nodes.length !== snap.nodes.length || prev.links.length !== snap.links.length
  const isFirst = prev.nodes.length === 0

  for (const n of snap.nodes) {
    const existing = prevNodes.get(n.id)
    if (existing) {
      // Drop optional fields the server no longer sends, then copy the new values.
      for (const k of ['controller', 'lag', 'state', 'members', 'partitions', 'leaders'] as const) {
        if (!(k in n)) delete existing[k]
      }
      Object.assign(existing, n)
      nodes.push(existing)
      prevNodes.delete(n.id)
    } else {
      structural = true
      const node: GraphNode = { ...n, bornAt: isFirst ? 0 : now }
      const anchor = n.group
        ? (prev.nodes.find((p) => p.id === `group:${n.group}`) as GraphNode | undefined)
        : undefined
      if (anchor?.x !== undefined) {
        node.x = anchor.x + (Math.random() - 0.5) * 10
        node.y = (anchor.y ?? 0) + (Math.random() - 0.5) * 10
        node.z = (anchor.z ?? 0) + (Math.random() - 0.5) * 10
      }
      nodes.push(node)
      if (!isFirst && (n.kind === 'member' || n.kind === 'group')) changes.push({ kind: 'joined', node: n, at: now })
    }
  }
  for (const gone of prevNodes.values()) {
    structural = true
    if (gone.kind === 'member' || gone.kind === 'group') changes.push({ kind: 'left', node: gone, at: now })
  }

  const prevLinks = new Map(prev.links.map((l) => [l.key, l]))
  const links: GraphLink[] = snap.links.map((l) => {
    const key = linkKey(l)
    const existing = prevLinks.get(key)
    if (existing) {
      existing.rate = l.rate
      existing.partitions = l.partitions
      return existing
    }
    structural = true
    return { ...l, key }
  })
  return { graph: { nodes, links }, structural, changes }
}

export type FilterOptions = { hideInternal: boolean; query: string }

/**
 * Returns the visible subgraph. A query keeps matching nodes plus their
 * neighbourhood (group ↔ members ↔ topics → brokers) without expanding out of
 * brokers, which would pull in everything.
 */
export function filterGraph(g: Graph, { hideInternal, query }: FilterOptions): Graph {
  let nodes = g.nodes
  if (hideInternal) nodes = nodes.filter((n) => !(n.kind === 'topic' && n.internal))
  const q = query.trim().toLowerCase()
  if (q) {
    const adj = new Map<string, Set<string>>()
    const add = (a: string, b: string) => {
      if (!adj.has(a)) adj.set(a, new Set())
      adj.get(a)?.add(b)
    }
    for (const l of g.links) {
      const s = endpointId(l.source)
      const t = endpointId(l.target)
      add(s, t)
      add(t, s)
    }
    const keep = new Set<string>()
    const queue = nodes.filter((n) => n.label.toLowerCase().includes(q)).map((n) => n.id)
    while (queue.length > 0) {
      const id = queue.shift() as string
      if (keep.has(id)) continue
      keep.add(id)
      if (id.startsWith('broker:')) continue
      for (const next of adj.get(id) ?? []) if (!keep.has(next)) queue.push(next)
    }
    nodes = nodes.filter((n) => keep.has(n.id))
  }
  if (nodes.length === g.nodes.length) return g
  const ids = new Set(nodes.map((n) => n.id))
  const links = g.links.filter((l) => ids.has(endpointId(l.source)) && ids.has(endpointId(l.target)))
  return { nodes, links }
}

export type GroupStatus = 'good' | 'warning' | 'serious' | 'critical'

/**
 * Lag status of a group: red when nobody consumes a lagging group. A group
 * that consumed recently (rate > 0) isn't abandoned even if it has no member
 * at this instant (consumers that reconnect between batches).
 */
export function groupStatus(n: Pick<SnapshotNode, 'lag' | 'members' | 'rate'>): GroupStatus {
  const lag = n.lag ?? 0
  if (lag === 0) return 'good'
  if (!n.members && !(n.rate > 0)) return 'critical'
  if (lag >= 10_000) return 'serious'
  return 'warning'
}

export const statusLabel: Record<GroupStatus, string> = {
  good: 'No lag',
  warning: 'Behind',
  serious: 'Far behind (≥ 10k)',
  critical: 'Lag with no consumers',
}

// ---- 2D flow view model: brokers on top, producers → topics → consumer groups ----

export type FlowEdge = { topicId: string; groupId: string; rate: number; partitions: number }
export type FlowTopic = { node: GraphNode; leaders: { broker: string; partitions: number }[]; outRate: number }
export type FlowGroup = { node: GraphNode; members: GraphNode[]; status: GroupStatus }
export type Flow = {
  brokers: GraphNode[]
  topics: FlowTopic[]
  groups: FlowGroup[]
  edges: FlowEdge[]
  maxRate: number
}

/**
 * Lays the graph out as columns. Topics are ordered by ingress, groups by the
 * first topic they read, so most edges run straight across.
 */
export function buildFlow(g: Graph): Flow {
  const byId = new Map(g.nodes.map((n) => [n.id, n]))
  const brokers = g.nodes
    .filter((n) => n.kind === 'broker')
    .sort((a, b) => a.label.localeCompare(b.label, undefined, { numeric: true }))
  const leaders = new Map<string, { broker: string; partitions: number }[]>()
  const membersOf = new Map<string, GraphNode[]>()
  const edges: FlowEdge[] = []
  for (const l of g.links) {
    const s = endpointId(l.source)
    const t = endpointId(l.target)
    if (l.kind === 'leader') {
      const b = byId.get(t)
      if (b) leaders.set(s, [...(leaders.get(s) ?? []), { broker: b.label, partitions: l.partitions }])
    } else if (l.kind === 'member') {
      const m = byId.get(t)
      if (m) membersOf.set(s, [...(membersOf.get(s) ?? []), m])
    } else if (l.kind === 'offsets' && byId.has(t)) {
      edges.push({ topicId: t, groupId: s, rate: l.rate, partitions: l.partitions })
    }
  }
  const outRate = new Map<string, number>()
  for (const e of edges) outRate.set(e.topicId, (outRate.get(e.topicId) ?? 0) + e.rate)

  const topics = g.nodes
    .filter((n) => n.kind === 'topic')
    .sort(
      (a, b) =>
        b.rate - a.rate || (outRate.get(b.id) ?? 0) - (outRate.get(a.id) ?? 0) || a.label.localeCompare(b.label),
    )
    .map((node) => ({
      node,
      leaders: (leaders.get(node.id) ?? []).sort((a, b) =>
        a.broker.localeCompare(b.broker, undefined, { numeric: true }),
      ),
      outRate: outRate.get(node.id) ?? 0,
    }))
  const topicIndex = new Map(topics.map((t, i) => [t.node.id, i]))
  const firstTopic = (groupId: string) =>
    Math.min(
      ...edges.filter((e) => e.groupId === groupId).map((e) => topicIndex.get(e.topicId) ?? Number.MAX_SAFE_INTEGER),
      Number.MAX_SAFE_INTEGER,
    )
  const groups = g.nodes
    .filter((n) => n.kind === 'group')
    .map((node) => ({
      node,
      members: (membersOf.get(node.id) ?? []).sort((a, b) => a.label.localeCompare(b.label)),
      status: groupStatus(node),
    }))
    .sort((a, b) => firstTopic(a.node.id) - firstTopic(b.node.id) || a.node.label.localeCompare(b.node.label))

  const maxRate = Math.max(1, ...topics.map((t) => Math.max(t.node.rate, t.outRate)), ...edges.map((e) => e.rate))
  return { brokers, topics, groups, edges: edges.filter((e) => topicIndex.has(e.topicId)), maxRate }
}

/** Stroke width for a rate: 1.5px when idle, up to 10px at the busiest flow (sqrt keeps small flows visible). */
export const flowWidth = (rate: number, max: number) => (rate <= 0 ? 1.5 : 2 + 8 * Math.sqrt(Math.min(rate / max, 1)))

/** Dash animation period: faster flows move faster; 0 = static. */
export const flowSeconds = (rate: number) => (rate <= 0 ? 0 : Math.max(0.25, Math.min(4, 4 / Math.log2(2 + rate))))
