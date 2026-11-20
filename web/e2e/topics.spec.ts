import { expect, test } from '@playwright/test'
import { cluster, uniq } from './helpers'

test('create a topic, produce, browse, delete', async ({ page }) => {
  const topic = uniq('e2e-pw')
  await page.goto(`/clusters/${cluster}/topics`)

  await page.getByRole('button', { name: 'Create topic' }).click()
  const create = page.getByRole('dialog')
  await create.getByLabel('Name').fill(topic)
  await create.getByLabel('Partitions').fill('2')
  await create.getByRole('button', { name: 'Create', exact: true }).click()
  await expect(page).toHaveURL(new RegExp(`/topics/${topic}$`))
  await expect(page.getByRole('heading', { name: topic })).toBeVisible()

  await page.getByRole('button', { name: 'Produce' }).click()
  const produce = page.getByRole('dialog')
  await produce.getByLabel('Value', { exact: true }).fill('{"hello":"playwright"}')
  await produce.getByRole('button', { name: 'Produce' }).click()
  await expect(page.getByText(/Produced to partition \d at offset 0/)).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(produce).toHaveCount(0)

  await page.getByRole('tab', { name: 'Messages' }).click()
  await page.getByRole('button', { name: 'Search' }).click()
  await expect(page.getByText(/reached the end|hit the result limit/)).toBeVisible()
  const row = page.getByRole('row').filter({ hasText: 'playwright' })
  await expect(row).toHaveCount(1)
  await row.click()
  await expect(page.getByRole('dialog').getByText('"hello": "playwright"')).toBeVisible()
  await page.keyboard.press('Escape')

  await page.getByRole('button', { name: 'Topic actions' }).click()
  await page.getByRole('menuitem', { name: 'Delete topic' }).click()
  const confirm = page.getByRole('alertdialog')
  await expect(confirm.getByRole('button', { name: 'Delete topic' })).toBeDisabled()
  await confirm.getByLabel(/Type .* to confirm/).fill(topic)
  await confirm.getByRole('button', { name: 'Delete topic' }).click()
  await expect(page).toHaveURL(new RegExp(`/clusters/${cluster}/topics$`))
  await expect(page.getByText(`Deleted ${topic}`)).toBeVisible()
})
