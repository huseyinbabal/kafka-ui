import { MutationCache, QueryCache, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { ThemeProvider } from 'next-themes'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { toast } from 'sonner'
import { Toaster } from '@/components/ui/sonner'
import { TooltipProvider } from '@/components/ui/tooltip'
import { meKey } from '@/features/auth/api'
import { ApiError } from '@/lib/api'
import { makeRouter } from '@/router'
import './index.css'

function onUnauthorized(err: unknown) {
  if (err instanceof ApiError && err.status === 401) {
    queryClient.setQueryData(meKey, null)
    void router.navigate({ to: '/login', search: { next: window.location.pathname } })
  }
}

const queryClient: QueryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5_000,
      retry: (count, err) =>
        !(err instanceof ApiError && (err.status < 500 || err.code === 'CLUSTER_OFFLINE')) && count < 2,
    },
  },
  queryCache: new QueryCache({ onError: onUnauthorized }),
  mutationCache: new MutationCache({
    onError: (err) => {
      onUnauthorized(err)
      if (!(err instanceof ApiError && err.status === 401)) toast.error(err.message)
    },
  }),
})

const router = makeRouter(queryClient)

// biome-ignore lint/style/noNonNullAssertion: root element is in index.html
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeProvider attribute="class" defaultTheme="dark" enableSystem disableTransitionOnChange>
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>
          <RouterProvider router={router} />
          <Toaster richColors />
        </TooltipProvider>
      </QueryClientProvider>
    </ThemeProvider>
  </StrictMode>,
)
