/** The cluster the specs run against; mutations only touch e2e- prefixed resources. */
export const cluster = process.env.KAFKAUI_E2E_CLUSTER ?? 'orders'

/** Session files written by global-setup.ts. */
export const asAdmin = { storageState: 'e2e/.auth/admin.json' }
export const asViewer = { storageState: 'e2e/.auth/viewer.json' }

export const uniq = (prefix: string) => `${prefix}-${Date.now().toString(36)}`
