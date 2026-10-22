import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider } from '@tanstack/react-router'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { FirstCommit } from '@/components/FirstCommit'
import { carryInputs } from '@/lib/snapshot'
import { loadNamespaces, syncDocumentLanguage } from '@/i18n'
import { syncTheme } from '@/lib/theme'
import { useAuthStore } from '@/stores/auth'
import { router } from './router'
import './styles/index.css'

// Bootstrap before the first render: theme attribute, <html lang>, and start fetching the
// always-needed `core` dictionary in parallel with the route chunks.
syncTheme()
syncDocumentLanguage()
const core = loadNamespaces(['core'])
// Signed-in users will land in the app shell: fetch React Query in parallel with the routes.
if (useAuthStore.getState().token) void import('@/lib/queryClient')

const container = document.getElementById('root')
if (!container) throw new Error('#root missing')

/**
 * Prerendered public routes (`scripts/prerender.mjs` → dist/index.html, dist/login/index.html, …) ship a
 * static snapshot of the page inside #root so the browser paints it before any JS runs (FCP/LCP on slow
 * mobile). The client does not hydrate — a snapshot taken from a client render can never match hydration
 * (`useId` values differ) — it waits for the matched route instead (loader data, route chunk,
 * dictionaries) so the first commit is the complete page, and `createRoot` swaps the snapshot for an
 * identical tree in one frame: no blank flash, no Suspense fallback, no layout shift.
 * The snapshot page loads the app through a tiny boot module only after its first paint has been reported
 * (FCP/LCP stay on the static paint); fields typed into before the swap are carried over.
 * `window.__GG_PRERENDER__` is set by the prerender script: same path, then it reads the snapshot.
 */
const prerendering = (window as { __GG_PRERENDER__?: boolean }).__GG_PRERENDER__ === true
const hasSnapshot = container.firstElementChild !== null

async function boot(root: HTMLElement) {
  if (hasSnapshot || prerendering) {
    // A failed loader still renders (error component); never keep the snapshot forever.
    await Promise.all([router.load(), core]).catch(() => undefined)
  } else {
    void core
  }
  const carried = hasSnapshot ? carryInputs(root) : null
  createRoot(root).render(
    <StrictMode>
      <ErrorBoundary>
        <FirstCommit root={root} carried={carried} prerendering={prerendering}>
          <RouterProvider router={router} />
        </FirstCommit>
      </ErrorBoundary>
    </StrictMode>,
  )
}

void boot(container)
