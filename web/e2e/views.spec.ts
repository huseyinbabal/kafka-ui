import { expect, test } from '@playwright/test'
import { cluster } from './helpers'

test('schema registry lists subjects and shows a diff', async ({ page }) => {
  await page.goto(`/clusters/${cluster}/schemas`)
  await expect(page.getByRole('heading', { name: 'Schema Registry' })).toBeVisible()
  await page.getByText('e2e-avro-value').click()
  await page.getByRole('combobox', { name: 'Compare with' }).click()
  await page.getByRole('option', { name: 'Compare with v1' }).click()
  await expect(page.getByText(/v1 → v2: \d+ changed lines/)).toBeVisible()
})

test('analytics renders charts', async ({ page }) => {
  await page.goto(`/clusters/${cluster}/analytics`)
  await expect(page.getByText('Messages in / s')).toBeVisible()
  await expect(page.locator('.recharts-surface').first()).toBeVisible()
})

test('live view shows brokers, topics and groups', async ({ page }) => {
  await page.goto(`/clusters/${cluster}/live`)
  await expect(page.getByRole('region', { name: 'Brokers' }).getByRole('button')).toHaveCount(3)
  await expect(page.getByText('controller')).toBeVisible()
  await expect(page.getByRole('button', { name: /e2e-messages/ })).toBeVisible()
  await expect(page.getByRole('main').getByText('Consumer groups', { exact: true })).toBeVisible()
  await expect(page.getByText(/updated \d/)).toBeVisible()
})
