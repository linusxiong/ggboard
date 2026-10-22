import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider } from '@tanstack/react-router'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { loadNamespaces, syncDocumentLanguage } from '@/i18n'
import { syncTheme } from '@/lib/theme'
import { useAuthStore } from '@/stores/auth'
import { router } from './router'
import './styles/index.css'

// Bootstrap before the first render: theme attribute, <html lang>, and start fetching the
// always-needed `core` dictionary in parallel with the route chunks.
syncTheme()
syncDocumentLanguage()
void loadNamespaces(['core'])
// Signed-in users will land in the app shell: fetch React Query in parallel with the routes.
if (useAuthStore.getState().token) void import('@/lib/queryClient')

const container = document.getElementById('root')
if (!container) throw new Error('#root missing')

createRoot(container).render(
  <StrictMode>
    <ErrorBoundary>
      <RouterProvider router={router} />
    </ErrorBoundary>
  </StrictMode>,
)
