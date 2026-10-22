#!/usr/bin/env node
/**
 * UI smoke crawl against a running ggboard (the Go binary serving dist/, seeded by scripts/e2e-full.sh):
 * visits every public, user and admin page in both locales, light + dark, desktop + phone width, and
 * reports console errors/warnings, page errors, failed requests, leaked i18n keys / placeholders and
 * horizontal overflow. Optional screenshots.
 *
 *   BASE=http://127.0.0.1:18090 node scripts/ui-smoke.mjs                 # report (exit 1 on findings)
 *   BASE=… USER_EMAIL=user@example.com node scripts/ui-smoke.mjs --quick  # zh-CN light desktop + phone only
 *   BASE=… node scripts/ui-smoke.mjs --shots=../docs/screenshots           # also write screenshots
 *
 * Needs a Chrome binary (CHROME_PATH, default: macOS Google Chrome) — puppeteer-core does not download one.
 */
import fs from 'node:fs'
import path from 'node:path'
import puppeteer from 'puppeteer-core'

const BASE = process.env.BASE ?? 'http://127.0.0.1:18090'
const CHROME =
  process.env.CHROME_PATH ?? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
const ADMIN = { email: process.env.ADMIN_EMAIL ?? 'admin@example.com', password: process.env.ADMIN_PASSWORD ?? 'Passw0rd!!' }
const USER = { email: process.env.USER_EMAIL ?? 'user@example.com', password: process.env.USER_PASSWORD ?? 'Passw0rd!!' }
const args = new Set(process.argv.slice(2))
const quick = args.has('--quick')
const only = [...args].find((a) => a.startsWith('--only='))?.slice(7)

async function api(method, p, body, token) {
  const res = await fetch(BASE + '/api/v1' + p, {
    method,
    headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  })
  const json = await res.json().catch(() => null)
  if (!res.ok) throw new Error(`${method} ${p} → ${res.status} ${JSON.stringify(json)}`)
  return json
}

async function session(who) {
  const { data } = await api('POST', '/passport/login', who)
  if (!data.token) throw new Error(`login ${who.email}: ${JSON.stringify(data)}`)
  return data
}

const user = await session(USER)
const admin = await session(ADMIN)
const orders = await api('GET', '/user/orders', undefined, user.token)
const tickets = await api('GET', '/user/tickets', undefined, user.token)
const tradeNo = orders.data[0]?.trade_no
const ticketId = tickets.data[0]?.id

const pages = {
  guest: ['/', '/login', '/register', '/forgot', '/nope-404'],
  user: [
    '/dashboard',
    '/shop',
    '/orders',
    tradeNo && `/orders/${tradeNo}`,
    '/subscription',
    '/profile',
    '/profile?tab=security',
    '/invite',
    '/tickets',
    ticketId && `/tickets/${ticketId}`,
    '/knowledge',
    '/notices',
  ].filter(Boolean),
  admin: [
    '/admin',
    '/admin/dashboard',
    '/admin/users',
    '/admin/plans',
    '/admin/groups',
    '/admin/nodes',
    '/admin/routes',
    '/admin/orders',
    '/admin/coupons',
    '/admin/payments',
    '/admin/tickets',
    '/admin/notices',
    '/admin/knowledge',
    '/admin/settings',
    ...['register', 'subscribe', 'order', 'invite', 'server', 'email', 'frontend', 'app', 'security'].map(
      (g) => `/admin/settings?group=${g}`,
    ),
    '/admin/rulesets',
    '/admin/system',
  ],
}

const variants = []
for (const locale of quick ? ['zh-CN'] : ['zh-CN', 'en'])
  for (const theme of quick ? ['light'] : ['light', 'dark'])
    for (const width of [1440, 375]) variants.push({ locale, theme, width })

// i18n leaks: a text node that is only a dotted key ("nodes.ps.tcpHeader", "common:actions.save") or an
// unreplaced {placeholder}.
const LEAK_SCRIPT = () => {
  const out = []
  const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT)
  const key = /^[a-z][\w-]*(:[\w-]+)?(\.[\w-]+)+$/
  const placeholder = /\{[a-zA-Z_]+\}/
  for (let n = walker.nextNode(); n; n = walker.nextNode()) {
    const t = n.textContent.trim()
    if (!t || n.parentElement?.closest('script,style,pre,code,textarea,[data-allow-raw]')) continue
    const host = /^[\w-]+(\.[\w-]+)*\.(com|org|net|io|top|cc|cn|dev|app)$/.test(t) || /^go\d/.test(t)
    if ((key.test(t) && !host && !t.includes('@')) || (placeholder.test(t) && !t.includes('://') && !/ · /.test(t))) out.push(t)
  }
  // a form label that is still the raw setting / field key ("passkey_rp_id")
  for (const l of document.querySelectorAll('label,[data-slot=label]')) {
    const t = l.textContent.trim()
    if (/^[a-z][a-z0-9]*(_[a-z0-9]+)+$/.test(t)) out.push(`label=${t}`)
  }
  for (const el of document.querySelectorAll('[aria-label],[placeholder],[title]')) {
    for (const a of ['aria-label', 'placeholder', 'title']) {
      const v = el.getAttribute(a)
      if (v && (key.test(v) || placeholder.test(v))) out.push(`${a}=${v}`)
    }
  }
  return [...new Set(out)].slice(0, 20)
}

const browser = await puppeteer.launch({
  executablePath: CHROME,
  headless: true,
  args: ['--no-first-run', '--disable-extensions'],
})
const findings = []
const shotsDir = [...args].find((a) => a.startsWith('--shots='))?.slice(8)

try {
  for (const [role, list] of Object.entries(pages)) {
    if (only && only !== role) continue
    for (const v of variants) {
      const ctx = await browser.createBrowserContext()
      const page = await ctx.newPage()
      await page.setViewport({ width: v.width, height: v.width < 768 ? 812 : 900, isMobile: v.width < 768, hasTouch: v.width < 768 })
      await page.emulateMediaFeatures([{ name: 'prefers-color-scheme', value: v.theme }])
      const sess = role === 'admin' ? admin : role === 'user' ? user : null
      await page.evaluateOnNewDocument(
        (s, locale) => {
          localStorage.setItem('gg-ui', JSON.stringify({ state: { theme: 'system', locale, sidebarCollapsed: false }, version: 1 }))
          if (s) localStorage.setItem('gg-auth', JSON.stringify({ state: { token: s.token, user: s.user }, version: 1 }))
        },
        sess,
        v.locale,
      )
      let current = ''
      const add = (kind, detail) => findings.push({ role, ...v, page: current, kind, detail })
      page.on('console', (m) => {
        if (m.type() === 'error' || m.type() === 'warn') add(`console.${m.type()}`, m.text().slice(0, 300))
      })
      page.on('pageerror', (e) => add('pageerror', String(e).slice(0, 300)))
      page.on('requestfailed', (r) => {
        if (!r.failure()?.errorText.includes('ERR_ABORTED')) add('requestfailed', `${r.url()} ${r.failure()?.errorText}`)
      })
      page.on('response', (r) => {
        const u = r.url()
        if (r.status() >= 400 && !(current === '/nope-404' && !u.includes('/api/'))) add(`http ${r.status()}`, u.replace(BASE, ''))
      })
      for (const p of list) {
        current = p
        await page.goto(BASE + p, { waitUntil: 'networkidle0', timeout: 30000 }).catch((e) => add('goto', String(e)))
        await new Promise((r) => setTimeout(r, 400))
        const landed = new URL(page.url()).pathname
        if (landed !== new URL(BASE + p).pathname) add('redirect', `→ ${landed}`)
        const leaks = await page.evaluate(LEAK_SCRIPT)
        if (leaks.length) add('i18n', leaks.join(' | '))
        const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
        if (overflow > 1) add('overflow-x', `${overflow}px`)
        const title = await page.title()
        if (!title || title === 'GG Bond' && p !== '/') add('title', JSON.stringify(title))
        if (shotsDir) {
          const name = `${role}${p.replace(/[/?=]+/g, '_')}-${v.locale}-${v.theme}-${v.width}.png`
          fs.mkdirSync(shotsDir, { recursive: true })
          await page.screenshot({ path: path.join(shotsDir, name) })
        }
      }
      await ctx.close()
    }
  }
} finally {
  await browser.close()
}

// Collapse identical findings across variants.
const grouped = new Map()
for (const f of findings) {
  const k = `${f.role} ${f.page} ${f.kind} ${f.detail}`
  const g = grouped.get(k) ?? { ...f, variants: [] }
  g.variants.push(`${f.locale}/${f.theme}/${f.width}`)
  grouped.set(k, g)
}
for (const g of grouped.values())
  console.log(`[${g.kind}] ${g.role} ${g.page} :: ${g.detail}  (${g.variants.length === variants.length ? 'all' : g.variants.join(',')})`)
console.log(`${grouped.size} distinct findings`)
process.exit(grouped.size ? 1 : 0)
