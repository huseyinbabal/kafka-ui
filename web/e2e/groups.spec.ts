import { expect, test } from '@playwright/test'
import { cluster } from './helpers'

// e2e-reader is a test group without members (created by the integration setup in kind).
test('reset offsets: preview first, then apply', async ({ page }) => {
  await page.goto(`/clusters/${cluster}/groups/e2e-reader`)
  await expect(page.getByRole('heading', { name: 'e2e-reader' })).toBeVisible()

  await page.getByRole('button', { name: 'Reset offsets' }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByRole('button', { name: 'Apply' })).toBeDisabled()
  await dialog.getByRole('button', { name: 'Preview' }).click()
  await expect(dialog.getByRole('row')).not.toHaveCount(0)
  await expect(dialog.getByRole('button', { name: 'Apply' })).toBeEnabled()
  await dialog.getByRole('button', { name: 'Apply' }).click()
  await expect(page.getByText(/Offsets of e2e-reader on .* were reset/)).toBeVisible()
})
