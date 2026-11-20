import { expect, test } from '@playwright/test'

/** 500 nodes: 3 brokers, 250 topics, 100 groups, 147 members, with traffic on every edge. */
function bigSnapshot() {
  const nodes: Record<string, unknown>[] = []
  const links: Record<string, unknown>[] = []
  for (let b = 0; b < 3; b++)
    nodes.push({ id: `broker:${b}`, kind: 'broker', label: `broker ${b}`, rate: 100, controller: b === 0 })
  for (let t = 0; t < 250; t++) {
    nodes.push({
      id: `topic:perf-topic-${t}`,
      kind: 'topic',
      label: `perf-topic-${t}`,
      rate: (t % 50) + 1,
      partitions: 6,
    })
    links.push({ source: `topic:perf-topic-${t}`, target: `broker:${t % 3}`, kind: 'leader', partitions: 6, rate: 1 })
  }
  let m = 0
  for (let g = 0; g < 100; g++) {
    nodes.push({
      id: `group:perf-group-${g}`,
      kind: 'group',
      label: `perf-group-${g}`,
      rate: 5,
      lag: g * 10,
      members: 1,
      state: 'Stable',
    })
    for (const t of [g, g + 100]) {
      links.push({
        source: `group:perf-group-${g}`,
        target: `topic:perf-topic-${t}`,
        kind: 'offsets',
        partitions: 6,
        rate: 3,
      })
    }
    const members = g < 47 ? 2 : 1
    for (let i = 0; i < members; i++, m++) {
      const id = `member:perf-group-${g}/m${i}`
      nodes.push({
        id,
        kind: 'member',
        label: `c${m}@10.0.0.${m % 255}`,
        clientId: `c${m}`,
        host: `10.0.0.${m % 255}`,
        group: `perf-group-${g}`,
        rate: 2,
        partitions: 3,
      })
      links.push({ source: `group:perf-group-${g}`, target: id, kind: 'member', partitions: 3, rate: 2 })
    }
  }
  return { at: Date.now(), nodes, links }
}

test('live view stays smooth with 500 nodes', async ({ page }) => {
  const snap = bigSnapshot()
  expect(snap.nodes).toHaveLength(500)
  await page.route('**/api/v1/clusters/*/live', (route) =>
    route.fulfill({
      status: 200,
      headers: { 'Content-Type': 'text/event-stream' },
      body: `event: hello\ndata: {"interval":2000}\n\nevent: snapshot\ndata: ${JSON.stringify(snap)}\n\n`,
    }),
  )
  const started = Date.now()
  await page.goto('/clusters/orders/live')
  await page.getByLabel('Hide internal topics').uncheck()
  await expect(page.getByText(/^perf-topic-\d+$/)).toHaveCount(250)
  await expect(page.getByText(/^perf-group-\d+$/)).toHaveCount(100)
  const renderMs = Date.now() - started

  const fps = await page.evaluate(
    () =>
      new Promise<number>((resolve) => {
        let frames = 0
        const t0 = performance.now()
        const tick = () => {
          frames++
          if (performance.now() - t0 < 3000) requestAnimationFrame(tick)
          else resolve(frames / 3)
        }
        requestAnimationFrame(tick)
      }),
  )
  console.log(`500 nodes: rendered in ${renderMs} ms, ${fps.toFixed(1)} fps`)
  expect(fps).toBeGreaterThan(30)
})
