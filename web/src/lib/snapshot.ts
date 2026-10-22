/** Values typed into a prerendered snapshot before the app replaced it (see components/FirstCommit.tsx). */
export interface Carried {
  values: Map<string, string>
  focused: string | null
}

export function carryInputs(root: HTMLElement): Carried {
  const values = new Map<string, string>()
  for (const el of root.querySelectorAll<HTMLInputElement | HTMLTextAreaElement>(
    'input[name], textarea[name]',
  )) {
    if (el.value && el.type !== 'hidden' && el.type !== 'checkbox' && el.type !== 'radio')
      values.set(el.name, el.value)
  }
  const active = document.activeElement
  const focused =
    (active instanceof HTMLInputElement || active instanceof HTMLTextAreaElement) && root.contains(active)
      ? active.name
      : null
  return { values, focused }
}

export function restoreInputs(root: HTMLElement, { values, focused }: Carried) {
  for (const [name, value] of values) {
    const el = root.querySelector(`[name="${CSS.escape(name)}"]`)
    if (!(el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement) || el.value) continue
    Object.getOwnPropertyDescriptor(Object.getPrototypeOf(el), 'value')?.set?.call(el, value)
    el.dispatchEvent(new Event('input', { bubbles: true }))
  }
  if (focused) root.querySelector<HTMLElement>(`[name="${CSS.escape(focused)}"]`)?.focus()
}
