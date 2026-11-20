import { chromium, type FullConfig } from '@playwright/test'

/**
 * Logs in once per role and saves the session, because the server rate-limits
 * logins (5/min per IP) and every spec logging in would trip it.
 */
export default async function globalSetup(config: FullConfig) {
  const baseURL = config.projects[0].use.baseURL as string
  const browser = await chromium.launch()
  for (const user of ['admin', 'viewer'] as const) {
    const page = await browser.newPage({ baseURL })
    await page.goto('/login')
    await page.getByLabel('Username').fill(user)
    await page.getByLabel('Password').fill(process.env[`KAFKAUI_E2E_${user.toUpperCase()}_PASSWORD`] ?? user)
    await page.getByRole('button', { name: 'Sign in' }).click()
    await page.waitForURL(/\/clusters\//)
    await page.context().storageState({ path: `e2e/.auth/${user}.json` })
    await page.close()
  }
  await browser.close()
}
