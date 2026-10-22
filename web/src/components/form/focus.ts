/**
 * Inline-validation helper: after a failed submit, scroll the first invalid control into view and
 * focus it. Scoped to the top-most open dialog when there is one. Never steals focus the user has
 * already moved somewhere else (MISTAKES #68/#86): it only acts if focus is where it was at submit.
 */
const INVALID =
  '[aria-invalid="true"], [data-invalid="true"] input:not([type="hidden"]), [data-invalid="true"] textarea, [data-invalid="true"] button'

export function focusFirstInvalid(scope?: ParentNode | null) {
  if (typeof document === 'undefined') return
  const before = document.activeElement
  requestAnimationFrame(() => {
    // One more task: React commits the error state after the store update that triggered this.
    window.setTimeout(() => {
      if (document.activeElement !== before) return
      const dialogs = document.querySelectorAll('[role="dialog"], [role="alertdialog"]')
      const root = scope ?? dialogs[dialogs.length - 1] ?? document
      const target = root.querySelector<HTMLElement>(INVALID)
      if (!target) return
      const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
      target.scrollIntoView?.({ block: 'center', behavior: reduce ? 'auto' : 'smooth' })
      target.focus({ preventScroll: true })
    }, 0)
  })
}
