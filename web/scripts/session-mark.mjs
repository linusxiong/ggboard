/**
 * Snapshot pages are built without a session, so they carry both variants of whatever depends on one
 * (`data-session-only`, see `routes/index.tsx`). This runs in the boot script before the first paint and lets
 * `<html data-session>` pick the variant. prerender.mjs inlines its source, so it cannot import `stores/auth.ts`:
 * it reads the same storage by the same rules, and `src/test/session-mark.test.ts` holds the two together.
 */
export function markSession() {
  let token
  try {
    token = JSON.parse(localStorage.getItem('gg-auth')).state.token
  } catch {
    return
  }
  if (typeof token !== 'string' || !token) return
  try {
    const { exp } = JSON.parse(atob(token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/')))
    if (typeof exp === 'number' && exp * 1000 <= Date.now()) return
  } catch {
    /* no readable expiry: the store keeps such a token too */
  }
  document.documentElement.dataset.session = 'member'
}

/**
 * A forced theme (`gg-ui`, see `stores/ui.ts`) has to be on `<html>` before the first paint as well: the app's own
 * module script runs after it, so the page would first show in the system scheme. Same DOM as `applyTheme` in
 * `src/lib/theme.ts` (`src/test/session-mark.test.ts` compares the two): `data-theme`, and both `theme-color` metas pinned.
 * Self-contained like `markSession`: prerender.mjs inlines its source.
 */
export function markTheme() {
  let theme
  try {
    theme = JSON.parse(localStorage.getItem('gg-ui')).state.theme
  } catch {
    return
  }
  if (theme !== 'light' && theme !== 'dark') return
  document.documentElement.dataset.theme = theme
  for (const meta of document.querySelectorAll('meta[name="theme-color"]')) {
    meta.dataset.media = meta.getAttribute('media') ?? ''
    meta.removeAttribute('media')
    meta.content = theme === 'dark' ? '#000000' : '#F2F2F7'
  }
}
