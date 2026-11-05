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
