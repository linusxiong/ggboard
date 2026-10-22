/**
 * Leaves the SPA for a payment gateway (checkout result `type: "url"`). Same tab, so the gateway's
 * return URL (`/guest/payment/return/:trade_no` → `/orders/:trade_no`) brings the user back here.
 * Only http(s) URLs are followed. A module function so tests can mock it (jsdom has no navigation).
 */
export function redirectToGateway(url: string): boolean {
  let target: URL
  try {
    target = new URL(url, window.location.href)
  } catch {
    return false
  }
  if (target.protocol !== 'https:' && target.protocol !== 'http:') return false
  window.location.assign(target.href)
  return true
}
