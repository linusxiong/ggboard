/**
 * lhci `puppeteerScript` for LH_TARGET=auth (see lighthouserc.cjs): signs in through the real API and stores
 * the session where the SPA reads it (`gg-auth` in localStorage) before Lighthouse loads a signed-in page.
 * Credentials: LH_USER / LH_PASSWORD (test account on the server under test; defaults match
 * scripts/e2e-full.sh seeds with USER_EMAIL=user@example.com).
 */
module.exports = async (browser, context) => {
  const origin = new URL(context.url).origin
  const email = process.env.LH_USER || 'user@example.com'
  const password = process.env.LH_PASSWORD || 'Passw0rd!!'
  const res = await fetch(`${origin}/api/v1/passport/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  })
  const body = await res.json()
  if (!res.ok || !body.data || !body.data.token) {
    throw new Error(`lh-login: sign-in failed (${res.status}): ${JSON.stringify(body)}`)
  }
  const page = await browser.newPage()
  // Any same-origin document gives access to the origin's localStorage; the plain shell is the lightest.
  await page.goto(`${origin}/spa.html`, { waitUntil: 'domcontentloaded' })
  await page.evaluate(
    (session) => {
      localStorage.setItem('gg-auth', JSON.stringify({ state: session, version: 1 }))
    },
    { token: body.data.token, user: body.data.user },
  )
  await page.close()
}
