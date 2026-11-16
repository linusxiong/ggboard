#!/usr/bin/env node
/**
 * Compare how two frontend builds render, element by element. For UI library updates: CI, the unit tests and
 * Lighthouse do not open a signed-in page or click anything, so a component our skin overrides can break without
 * failing them (MISTAKES #313).
 *
 * Serve the old build and the new one from the SAME throwaway panel (same data), one after the other:
 *
 *   GGBOARD_FRONTEND_DIR=/path/to/old/dist ggboard serve      # seeded by scripts/e2e-full.sh (KEEP=1)
 *   BASE=http://127.0.0.1:18090 node scripts/ui-diff.mjs snapshot /tmp/before.json
 *   GGBOARD_FRONTEND_DIR=$PWD/dist ggboard serve               # restart: the server indexes dist/ at start
 *   BASE=http://127.0.0.1:18090 node scripts/ui-diff.mjs snapshot /tmp/after.json
 *   node scripts/ui-diff.mjs diff /tmp/before.json /tmp/after.json
 *
 * A snapshot holds, for every page at desktop and phone width in light and dark: the box and the computed style of
 * every element; and, for the menus, list boxes and popovers it can open: the same, plus the names of the animations
 * that started. Take two snapshots of one build first: their diff has to be empty, otherwise a page has live
 * content (a log, a clock) that you then know to ignore. A snapshot takes about five minutes.
 *
 * It changes no data: the only elements it clicks are the ones that announce a popup (`aria-haspopup`), and it
 * closes each with Escape. Dialogs opened by a plain button, drawers and switches are not covered: open and toggle
 * those by hand. Needs CHROME_PATH (see ui-smoke.mjs) and an administrator without 2FA (ADMIN_EMAIL /
 * ADMIN_PASSWORD, default: the e2e-full.sh account).
 */
import fs from 'node:fs'
import puppeteer from 'puppeteer-core'

const PROPS = [
  'display',
  'color',
  'background-color',
  'border-top-color',
  'border-top-width',
  'border-top-left-radius',
  'border-bottom-right-radius',
  'box-shadow',
  'opacity',
  'font-size',
  'font-weight',
  'line-height',
  'padding-top',
  'padding-left',
  'gap',
  'transition-duration',
  'transition-timing-function',
  'animation-name',
  'clip-path',
  'backdrop-filter',
  'fill',
  'stroke',
]
const PAGES = {
  guest: ['/', '/login', '/register', '/forgot'],
  signedIn: [
    '/dashboard',
    '/shop',
    '/orders',
    '/subscription',
    '/profile',
    '/profile?tab=security',
    '/invite',
    '/tickets',
    '/knowledge',
    '/notices',
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
    '/admin/mail-templates',
    '/admin/rulesets',
  ],
}
const VIEWPORTS = {
  desktop: { width: 1280, height: 900 },
  phone: { width: 390, height: 844, isMobile: true, hasTouch: true },
}
const MAX_OVERLAYS = 4
const MAX_TRIGGERS = 8
const OVERLAY_ROLES = ['dialog', 'alertdialog', 'menu', 'listbox']
const OVERLAY = OVERLAY_ROLES.map((role) => `[role=${role}]`).join(',')
/** React Aria portals overlays to the body, next to the app root. */
const PORTALED_OVERLAY = OVERLAY_ROLES.map((role) => `body > :not(#root) [role=${role}]`).join(',')
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

/** Waits until the animations and transitions that end have ended: a box measured mid-animation is noise. */
async function settle(page) {
  await sleep(100)
  await page.evaluate(() =>
    Promise.race([
      Promise.all(
        document
          .getAnimations()
          .filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity)
          .map((animation) => animation.finished.catch(() => {})),
      ),
      new Promise((resolve) => setTimeout(resolve, 2000)),
    ]),
  )
  await sleep(100)
}

/** Runs in the page: box and computed style of `roots` and everything below them. */
function collect(props, selector) {
  const key = (el) => {
    const parts = []
    for (let n = el; n && n !== document.body; n = n.parentElement) {
      parts.unshift(`${n.tagName.toLowerCase()}:${[...n.parentElement.children].indexOf(n)}`)
    }
    return parts.join('>')
  }
  const roots = selector
    ? [...document.querySelectorAll(selector)].filter((el) => !el.closest('main'))
    : [document.body]
  const rows = {}
  for (const root of roots) {
    const elements = [root, ...root.querySelectorAll('*')]
    // The wrappers a UI library positions and animates (popover, modal container, backdrop) sit above the role.
    if (selector)
      for (let p = root.parentElement; p && p !== document.body; p = p.parentElement) elements.push(p)
    for (const el of elements) {
      if (['SCRIPT', 'STYLE', 'LINK', 'META'].includes(el.tagName)) continue
      const style = getComputedStyle(el)
      const box = el.getBoundingClientRect()
      const cls = typeof el.className === 'string' ? el.className : (el.className.baseVal ?? '')
      rows[key(el)] = {
        cls: cls.split(/\s+/).slice(0, 3).join(' ').slice(0, 80) || el.tagName.toLowerCase(),
        box: [box.x, box.y, box.width, box.height].map((n) => Math.round(n * 2) / 2),
        style: props.map((p) => style.getPropertyValue(p)),
      }
    }
  }
  return rows
}

async function snapshot(file) {
  const base = process.env.BASE ?? 'http://127.0.0.1:18090'
  const chrome = process.env.CHROME_PATH ?? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
  const res = await fetch(`${base}/api/v1/passport/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      email: process.env.ADMIN_EMAIL ?? 'admin@example.com',
      password: process.env.ADMIN_PASSWORD ?? 'Passw0rd!!',
    }),
  })
  const session = (await res.json()).data
  if (!session?.token) throw new Error(`sign-in failed (${res.status})`)

  const browser = await puppeteer.launch({ executablePath: chrome })
  const out = { props: PROPS, pages: {}, overlays: {} }
  const open = async (path, signedIn, viewport, scheme, motion) => {
    const page = await browser.newPage()
    await page.setViewport(VIEWPORTS[viewport])
    await page.emulateMediaFeatures([
      { name: 'prefers-color-scheme', value: scheme },
      { name: 'prefers-reduced-motion', value: motion },
    ])
    if (signedIn) {
      await page.evaluateOnNewDocument(
        (state) => localStorage.setItem('gg-auth', JSON.stringify({ state, version: 1 })),
        { token: session.token, user: session.user },
      )
    }
    await page.goto(base + path, { waitUntil: 'networkidle0' })
    await sleep(300)
    return page
  }
  try {
    for (const viewport of Object.keys(VIEWPORTS)) {
      for (const [group, paths] of Object.entries(PAGES)) {
        for (const path of paths) {
          for (const scheme of ['light', 'dark']) {
            const page = await open(path, group === 'signedIn', viewport, scheme, 'reduce')
            out.pages[`${viewport} ${scheme} ${path}`] = await page.evaluate(collect, PROPS, null)
            await page.close()
          }
          if (group !== 'signedIn') continue
          // Overlays, with motion on so that the animations they start can be compared too.
          const page = await open(path, true, viewport, 'light', 'no-preference')
          await page.evaluate(() => {
            window.__animations = []
            document.addEventListener(
              'animationstart',
              (e) => window.__animations.push(e.animationName),
              true,
            )
          })
          const triggers = await page.$$('main [aria-haspopup]')
          let opened = 0
          for (let i = 0; i < triggers.length && i < MAX_TRIGGERS && opened < MAX_OVERLAYS; i++) {
            const label = await triggers[i].evaluate((el) =>
              (el.getAttribute('aria-label') || el.textContent || '').trim().slice(0, 30),
            )
            await page.evaluate(() => (window.__animations.length = 0))
            try {
              await triggers[i].click({ delay: 10 })
            } catch {
              continue // detached or covered
            }
            const appeared = await page.waitForSelector(PORTALED_OVERLAY, { timeout: 500 }).then(
              () => true,
              () => false,
            )
            if (!appeared) continue
            await settle(page)
            const rows = await page.evaluate(collect, PROPS, OVERLAY)
            if (!Object.keys(rows).length) continue
            opened++
            await page.keyboard.press('Escape')
            await settle(page)
            const animations = await page.evaluate(() => [...new Set(window.__animations)].sort())
            out.overlays[`${viewport} ${path} :: ${i} ${label}`] = { rows, animations }
          }
          await page.close()
        }
      }
    }
  } finally {
    await browser.close()
  }
  fs.writeFileSync(file, JSON.stringify(out))
  const elements = Object.values(out.pages).reduce((n, rows) => n + Object.keys(rows).length, 0)
  console.log(
    `${Object.keys(out.pages).length} page views (${elements} elements), ${Object.keys(out.overlays).length} overlays → ${file}`,
  )
}

function diff(fileA, fileB) {
  const a = JSON.parse(fs.readFileSync(fileA, 'utf8'))
  const b = JSON.parse(fs.readFileSync(fileB, 'utf8'))
  const groups = new Map()
  const add = (what, where) => groups.set(what, [...(groups.get(what) ?? []), where])
  let compared = 0
  let unmatched = 0
  const compare = (rowsA, rowsB, where) => {
    for (const [key, ra] of Object.entries(rowsA)) {
      const rb = rowsB[key]
      if (!rb) {
        unmatched++
        continue
      }
      compared++
      const d = ra.box.map((n, i) => Math.round((rb.box[i] - n) * 10) / 10)
      // A popover can land one pixel off between two runs of the same build.
      if (d.some((n) => Math.abs(n) > 1)) add(`.${ra.cls}  box x${d[0]} y${d[1]} w${d[2]} h${d[3]}`, where)
      a.props.forEach((prop, i) => {
        if (ra.style[i] !== rb.style[i]) {
          add(`.${ra.cls}  ${prop}: ${ra.style[i].slice(0, 60)} → ${rb.style[i].slice(0, 60)}`, where)
        }
      })
    }
  }
  for (const [where, rows] of Object.entries(a.pages)) compare(rows, b.pages[where] ?? {}, where)
  for (const [where, overlay] of Object.entries(a.overlays)) {
    const other = b.overlays[where]
    if (!other) {
      add('overlay opened in the first snapshot only', where)
      continue
    }
    compare(overlay.rows, other.rows, where)
    if (overlay.animations.join() !== other.animations.join()) {
      add(
        `animations: ${overlay.animations.join(', ') || 'none'} → ${other.animations.join(', ') || 'none'}`,
        where,
      )
    }
  }
  console.log(
    `${compared} elements compared, ${unmatched} without a counterpart, ${groups.size} kinds of difference`,
  )
  for (const [what, where] of [...groups].sort((x, y) => y[1].length - x[1].length)) {
    console.log(
      `${String(where.length).padStart(5)}×  ${what}\n        ${[...new Set(where)].slice(0, 4).join(' | ')}`,
    )
  }
}

const [command, ...files] = process.argv.slice(2)
if (command === 'snapshot' && files.length === 1) await snapshot(files[0])
else if (command === 'diff' && files.length === 2) diff(files[0], files[1])
else {
  console.error('usage: ui-diff.mjs snapshot <out.json> | ui-diff.mjs diff <before.json> <after.json>')
  process.exit(2)
}
