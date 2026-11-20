import { expect, test } from '@playwright/test'
import { asViewer, cluster } from './helpers'

test.describe('anonymous', () => {
  test.use({ storageState: { cookies: [], origins: [] } })

  test('wrong password is rejected', async ({ page }) => {
    await page.goto('/login')
    await page.getByLabel('Username').fill('admin')
    await page.getByLabel('Password').fill('definitely-wrong')
    await page.getByRole('button', { name: 'Sign in' }).click()
    await expect(page.getByText(/invalid username or password/i)).toBeVisible()
  })

  test('unauthenticated pages redirect to login', async ({ page }) => {
    await page.goto(`/clusters/${cluster}/topics`)
    await expect(page).toHaveURL(/\/login/)
  })
})

test.describe('viewer', () => {
  test.use(asViewer)

  test('sees no write actions', async ({ page }) => {
    await page.goto(`/clusters/${cluster}/topics`)
    await expect(page.getByRole('heading', { name: 'Topics' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Create topic' })).toHaveCount(0)
    await page.goto(`/clusters/${cluster}/topics/e2e-messages`)
    await expect(page.getByRole('heading', { name: 'e2e-messages' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Produce' })).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Topic actions' })).toHaveCount(0)
  })
})
