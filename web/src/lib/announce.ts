/**
 * Screen-reader announcements for things that appear without moving focus (toasts, "copied").
 *
 * `document.ariaNotify()` where the browser has it (Chrome 141, Firefox 150, Safari 27). Otherwise the message is
 * added to a polite live region that exists from the first render (`<LiveRegion>` in the root route): a region
 * that is inserted together with its text is often not read at all. Messages are separate nodes, so two in a row
 * (or the same one twice) are each announced, and they are removed again so nobody finds stale text later.
 */
export const LIVE_REGION_ID = 'gg-live'
const KEEP_MS = 10_000

export function announce(message: string) {
  const text = message.trim()
  if (!text) return
  const doc = document as Document & { ariaNotify?: (announcement: string) => void }
  if (typeof doc.ariaNotify === 'function') {
    doc.ariaNotify(text)
    return
  }
  const region = document.getElementById(LIVE_REGION_ID)
  if (!region) return
  const node = document.createElement('div')
  node.textContent = text
  region.append(node)
  window.setTimeout(() => node.remove(), KEEP_MS)
}
