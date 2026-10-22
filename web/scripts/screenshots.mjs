#!/usr/bin/env node
/**
 * README screenshots (docs/screenshots/*.png, 1440×900) from a running, seeded ggboard:
 *
 *   USER_EMAIL=user@example.com BASE=http://127.0.0.1:18090 ./scripts/e2e-full.sh   # seed (from the repo root)
 *   BASE=http://127.0.0.1:18090 node scripts/screenshots.mjs [outDir]               # default ../docs/screenshots
 *
 * Signs in through the API (ADMIN_EMAIL / USER_EMAIL, password ADMIN_PASSWORD / USER_PASSWORD), opens each
 * scene in light and/or dark, waits for idle-time animations and writes PNGs. A pending order is created for
 * the order-page scene and cancelled afterwards. Needs Chrome (CHROME_PATH).
 */
import fs from 'node:fs'
import path from 'node:path'
import puppeteer from 'puppeteer-core'

const BASE = process.env.BASE ?? 'http://127.0.0.1:18090'
const OUT = path.resolve(process.argv[2] ?? path.join(import.meta.dirname, '..', '..', 'docs', 'screenshots'))
const CHROME = process.env.CHROME_PATH ?? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
const ADMIN = {
  email: process.env.ADMIN_EMAIL ?? 'admin@example.com',
  password: process.env.ADMIN_PASSWORD ?? 'Passw0rd!!',
}
const USER = {
  email: process.env.USER_EMAIL ?? 'user@example.com',
  password: process.env.USER_PASSWORD ?? 'Passw0rd!!',
}

async function api(method, p, body, token) {
  const res = await fetch(`${BASE}/api/v1${p}`, {
    method,
    headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  })
  const json = await res.json().catch(() => null)
  if (!res.ok) throw new Error(`${method} ${p} → ${res.status} ${JSON.stringify(json)}`)
  return json.data
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
const user = await api('POST', '/passport/login', USER)
const admin = await api('POST', '/passport/login', ADMIN)
if (!user.token || !admin.token) throw new Error('sign-in needs accounts without 2FA')

// A pending order for the order page (cancelled at the end).
const plans = await api('GET', '/user/plans', undefined, user.token)
const plan = plans.find((p) => p.prices?.monthly) ?? plans[0]
let pending = null
try {
  pending = await api(
    'POST',
    '/user/orders',
    { plan_id: plan.id, period: Object.keys(plan.prices)[0] },
    user.token,
  )
} catch (error) {
  console.warn(`no pending order (${error.message}); the order scene uses the latest order`)
}
const orders = await api('GET', '/user/orders', undefined, user.token)
const tradeNo = pending?.trade_no ?? orders[0]?.trade_no

/** scene: name, who, path, themes, optional async prepare(page). */
const scenes = [
  { name: 'landing', who: null, path: '/', themes: ['light', 'dark'] },
  { name: 'login', who: null, path: '/login', themes: ['light', 'dark'] },
  { name: 'dashboard', who: user, path: '/dashboard', themes: ['light', 'dark'] },
  {
    name: 'subscription-rules',
    who: user,
    path: '/subscription',
    themes: ['light', 'dark'],
    prepare: async (page) => {
      await page.evaluate(() => {
        const heading = [...document.querySelectorAll('h2')].find((h) =>
          /分流规则|Routing rules/.test(h.textContent ?? ''),
        )
        heading?.scrollIntoView({ block: 'start' })
        window.scrollBy(0, -72)
      })
    },
  },
  { name: 'shop', who: user, path: '/shop', themes: ['light', 'dark'] },
  { name: 'order', who: user, path: `/orders/${tradeNo}`, themes: ['light'] },
  { name: 'admin-dashboard', who: admin, path: '/admin/dashboard', themes: ['light', 'dark'] },
  {
    name: 'admin-node-editor',
    who: admin,
    path: '/admin/nodes',
    themes: ['light', 'dark'],
    prepare: async (page) => {
      const edit = await page.waitForSelector(
        'button[aria-label^="编辑节点"], button[aria-label^="Edit node"]',
      )
      await edit.click()
      await page.waitForSelector('[role="dialog"] [role="tab"]')
      await sleep(400)
      const tabs = await page.$$('[role="dialog"] [role="tab"]')
      await tabs[1]?.click() // protocol settings
    },
  },
  { name: 'admin-settings', who: admin, path: '/admin/settings', themes: ['light'] },
]

fs.mkdirSync(OUT, { recursive: true })
const browser = await puppeteer.launch({
  executablePath: CHROME,
  headless: true,
  args: ['--no-first-run', '--hide-scrollbars'],
})
try {
  for (const scene of scenes) {
    for (const theme of scene.themes) {
      const ctx = await browser.createBrowserContext()
      const page = await ctx.newPage()
      await page.setViewport({ width: 1440, height: 900, deviceScaleFactor: 1 })
      await page.emulateMediaFeatures([
        { name: 'prefers-color-scheme', value: theme },
        { name: 'prefers-reduced-motion', value: 'no-preference' },
      ])
      await page.evaluateOnNewDocument(
        (session) => {
          localStorage.setItem(
            'gg-ui',
            JSON.stringify({
              state: { theme: 'system', locale: 'zh-CN', sidebarCollapsed: false },
              version: 1,
            }),
          )
          if (session) localStorage.setItem('gg-auth', JSON.stringify({ state: session, version: 1 }))
        },
        scene.who ? { token: scene.who.token, user: scene.who.user } : null,
      )
      await page.goto(BASE + scene.path, { waitUntil: 'networkidle0' })
      await sleep(1200) // idle-time animations (effects) and count-ups
      if (scene.prepare) {
        await scene.prepare(page)
        await sleep(900)
      }
      // No transient toasts in the README.
      await page.evaluate(() =>
        document.querySelectorAll('[data-slot="toast-region"]').forEach((t) => t.remove()),
      )
      const file = path.join(OUT, `${scene.name}-${theme}.png`)
      await page.screenshot({ path: file, type: 'png' })
      console.log(`${path.relative(process.cwd(), file)} ${(fs.statSync(file).size / 1024).toFixed(0)} KB`)
      await ctx.close()
    }
  }
} finally {
  await browser.close()
  if (pending)
    await api('POST', `/user/orders/${pending.trade_no}/cancel`, undefined, user.token).catch(() => {})
}
