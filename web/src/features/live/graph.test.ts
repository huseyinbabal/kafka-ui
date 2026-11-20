import { describe, expect, it } from 'vitest'
import { emptyGraph, filterGraph, type GraphNode, groupStatus, mergeSnapshot, type Snapshot } from './graph'

const snap = (members: string[], lag = 0): Snapshot => ({
  at: 1,
  nodes: [
    { id: 'broker:0', kind: 'broker', label: 'broker 0', rate: 0, controller: true },
    { id: 'topic:clicks', kind: 'topic', label: 'clicks', rate: 5, partitions: 2 },
    { id: 'topic:__consumer_offsets', kind: 'topic', label: '__consumer_offsets', rate: 0, internal: true },
    { id: 'topic:orders', kind: 'topic', label: 'orders', rate: 0 },
    { id: 'group:analytics', kind: 'group', label: 'analytics', rate: 3, lag, members: members.length },
    ...members.map((m) => ({
      id: `member:analytics/${m}`,
      kind: 'member' as const,
      label: m,
      rate: 1,
      group: 'analytics',
    })),
  ],
  links: [
    { source: 'topic:clicks', target: 'broker:0', kind: 'leader', partitions: 2, rate: 5 },
    { source: 'topic:orders', target: 'broker:0', kind: 'leader', partitions: 1, rate: 0 },
    { source: 'topic:__consumer_offsets', target: 'broker:0', kind: 'leader', partitions: 50, rate: 0 },
    ...members.flatMap((m) => [
      { source: 'group:analytics', target: `member:analytics/${m}`, kind: 'member' as const, partitions: 1, rate: 1 },
      { source: `member:analytics/${m}`, target: 'topic:clicks', kind: 'consumes' as const, partitions: 1, rate: 1 },
    ]),
  ],
})

describe('mergeSnapshot', () => {
  it('keeps node identity and positions across snapshots', () => {
    const first = mergeSnapshot(emptyGraph, snap(['a', 'b']), 100)
    expect(first.changes).toEqual([]) // the initial load is not a "join"
    const broker = first.graph.nodes.find((n) => n.id === 'broker:0') as GraphNode
    broker.x = 42
    const second = mergeSnapshot(first.graph, { ...snap(['a', 'b'], 7), at: 2 }, 200)
    expect(second.structural).toBe(false)
    const again = second.graph.nodes.find((n) => n.id === 'broker:0')
    expect(again).toBe(broker)
    expect(again?.x).toBe(42)
    expect(second.graph.nodes.find((n) => n.id === 'group:analytics')?.lag).toBe(7)
    expect(second.graph.links[0]).toBe(first.graph.links[0])
  })

  it('reports joins and leaves and seeds new members next to their group', () => {
    const first = mergeSnapshot(emptyGraph, snap(['a', 'b']), 100)
    const group = first.graph.nodes.find((n) => n.id === 'group:analytics') as GraphNode
    Object.assign(group, { x: 100, y: 100, z: 100 })
    const scaled = mergeSnapshot(first.graph, snap(['a', 'b', 'c']), 200)
    expect(scaled.structural).toBe(true)
    expect(scaled.changes.map((c) => `${c.kind}:${c.node.label}`)).toEqual(['joined:c'])
    const c = scaled.graph.nodes.find((n) => n.label === 'c') as GraphNode
    expect(c.bornAt).toBe(200)
    expect(Math.abs((c.x ?? 0) - 100)).toBeLessThan(10)
    const back = mergeSnapshot(scaled.graph, snap(['a', 'b']), 300)
    expect(back.changes.map((x) => `${x.kind}:${x.node.label}`)).toEqual(['left:c'])
  })

  it('drops optional fields the server stopped sending', () => {
    const first = mergeSnapshot(emptyGraph, snap(['a'], 5), 1)
    const next = mergeSnapshot(first.graph, snap(['a'], 0), 2)
    // lag 0 is sent as a missing field by the server (omitempty)
    const s = snap(['a'])
    delete s.nodes[4].lag
    const cleared = mergeSnapshot(next.graph, s, 3)
    expect(cleared.graph.nodes.find((n) => n.id === 'group:analytics')?.lag).toBeUndefined()
  })
})

describe('filterGraph', () => {
  const g = mergeSnapshot(emptyGraph, snap(['a', 'b']), 1).graph

  it('hides internal topics and their links', () => {
    const f = filterGraph(g, { hideInternal: true, query: '' })
    expect(f.nodes.some((n) => n.internal)).toBe(false)
    expect(f.links.some((l) => l.source === 'topic:__consumer_offsets')).toBe(false)
  })

  it('keeps the neighbourhood of a match without expanding through brokers', () => {
    const f = filterGraph(g, { hideInternal: false, query: 'analytics' })
    const ids = f.nodes.map((n) => n.id).sort()
    expect(ids).toEqual(['broker:0', 'group:analytics', 'member:analytics/a', 'member:analytics/b', 'topic:clicks'])
  })

  it('returns the same graph object when nothing is filtered', () => {
    expect(filterGraph(g, { hideInternal: false, query: '' })).toBe(g)
  })
})

describe('groupStatus', () => {
  it('maps lag and members to a status', () => {
    expect(groupStatus({ lag: 0, members: 0 })).toBe('good')
    expect(groupStatus({ lag: 5, members: 0 })).toBe('critical')
    expect(groupStatus({ lag: 5, members: 2 })).toBe('warning')
    expect(groupStatus({ lag: 50_000, members: 2 })).toBe('serious')
  })
})
