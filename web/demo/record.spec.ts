import { type Page, test } from '@playwright/test'

/**
 * A guided tour for the README. Run against a cluster with traffic
 * (`make kind-traffic`), then `make demo` turns the video into docs/public/demo.gif.
 */
const cluster = process.env.KAFKAUI_E2E_CLUSTER ?? 'orders'
const pause = (page: Page, ms: number) => page.waitForTimeout(ms)

test('demo', async ({ page }) => {
  await page.goto('/login')
  await pause(page, 600)
  await page.getByLabel('Username').pressSequentially('admin', { delay: 70 })
  await page.getByLabel('Password').pressSequentially('admin', { delay: 70 })
  await page.getByRole('button', { name: 'Sign in' }).click()
  await page.waitForURL(/\/clusters\//)
  await pause(page, 2200)

  await page.getByRole('link', { name: 'Topics' }).click()
  await page.getByText('e2e-traffic', { exact: true }).waitFor()
  await pause(page, 1500)
  await page.getByText('e2e-messages', { exact: true }).click()
  await page.getByRole('tab', { name: 'Messages' }).click()
  await pause(page, 600)
  await page.getByRole('button', { name: 'Search' }).click()
  await page.getByText(/hit the result limit|reached the end/).waitFor()
  await pause(page, 1200)
  await page.getByRole('row').filter({ hasText: '"status":"PAID"' }).first().click()
  await pause(page, 2500)
  await page.keyboard.press('Escape')

  await page.getByRole('link', { name: 'Consumer groups' }).click()
  await page.getByText('e2e-slow-consumer', { exact: true }).waitFor()
  await pause(page, 1200)
  await page.getByText('e2e-slow-consumer', { exact: true }).click()
  await page.getByText('Lag trend').waitFor()
  await pause(page, 2500)

  await page.getByRole('link', { name: 'Analytics' }).click()
  await page.getByText('Messages in / s').waitFor()
  await pause(page, 2500)
  await page.mouse.wheel(0, 650)
  await pause(page, 2000)

  await page.getByRole('link', { name: 'Live topology' }).click()
  await page.getByText(/updated \d/).waitFor()
  await pause(page, 6000)
  await page.getByLabel('Filter by topic, group or consumer').pressSequentially('e2e-traffic', { delay: 60 })
  await pause(page, 6000)
})
