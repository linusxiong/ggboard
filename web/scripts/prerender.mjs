#!/usr/bin/env node
/**
 * Build-time prerendering of the public routes — `pnpm build` runs it after `vite build`.
 *
 *  1. starts `vite preview --mode lighthouse` (fixture API = a fresh install's guest config, scripts/fixtures);
 *  2. opens each route in headless Chrome (puppeteer-core + the system Chrome) with
 *     `window.__GG_PRERENDER__ = true`: main.tsx waits for the matched route (loader data, chunk,
 *     dictionaries), renders once and hands over the first committed markup (`__GG_SNAPSHOT__`), i.e. the
 *     page before any idle-time animation, lazy section or toast;
 *  3. writes the snapshot into the Vite shell: dist/index.html (`/`), dist/login/index.html, … plus .br/.gz.
 *     The untouched shell is kept as dist/spa.html: the Go server (internal/static) and `vite preview`
 *     serve it for every other path, so deep links (/dashboard, /admin/…) never show the landing snapshot.
 *
 * The client never hydrates (see main.tsx): createRoot replaces the snapshot with an identical tree in one
 * commit. Without a Chrome binary (CHROME_PATH, or the usual install paths) the step is skipped with a
 * warning and dist/ stays a plain SPA (e.g. container builds without Chrome). PRERENDER=0 skips it too.
 */
import { spawn, execFileSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import fs from 'node:fs'
import net from 'node:net'
import path from 'node:path'
import zlib from 'node:zlib'

const ROOT = path.resolve(import.meta.dirname, '..')
const DIST = path.join(ROOT, 'dist')
/** route → output file (relative to dist). */
const ROUTES = {
  '/': 'index.html',
  '/login': 'login/index.html',
  '/register': 'register/index.html',
  '/forgot': 'forgot/index.html',
}
const SHELL = 'spa.html'

function log(msg) {
  console.log(`[prerender] ${msg}`)
}

function findChrome() {
  const candidates = [
    process.env.CHROME_PATH,
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    '/Applications/Chromium.app/Contents/MacOS/Chromium',
    '/usr/bin/google-chrome',
    '/usr/bin/google-chrome-stable',
    '/usr/bin/chromium',
    '/usr/bin/chromium-browser',
  ].filter(Boolean)
  for (const c of candidates) if (fs.existsSync(c)) return c
  for (const bin of ['google-chrome', 'chromium', 'chromium-browser']) {
    try {
      const p = execFileSync('which', [bin], { encoding: 'utf8' }).trim()
      if (p) return p
    } catch {
      /* not installed */
    }
  }
  return null
}

function freePort() {
  return new Promise((resolve, reject) => {
    const srv = net.createServer()
    srv.listen(0, '127.0.0.1', () => {
      const { port } = srv.address()
      srv.close(() => resolve(port))
    })
    srv.on('error', reject)
  })
}

async function waitFor(url, ms = 30000) {
  const end = Date.now() + ms
  while (Date.now() < end) {
    try {
      const res = await fetch(url)
      if (res.ok) return
    } catch {
      /* not up yet */
    }
    await new Promise((r) => setTimeout(r, 150))
  }
  throw new Error(`preview server did not start: ${url}`)
}

function escapeHtml(text) {
  return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

function writeCompressed(file, html) {
  fs.mkdirSync(path.dirname(file), { recursive: true })
  const buf = Buffer.from(html)
  fs.writeFileSync(file, buf)
  fs.writeFileSync(
    `${file}.br`,
    zlib.brotliCompressSync(buf, {
      params: {
        [zlib.constants.BROTLI_PARAM_QUALITY]: 11,
        [zlib.constants.BROTLI_PARAM_SIZE_HINT]: buf.length,
      },
    }),
  )
  fs.writeFileSync(`${file}.gz`, zlib.gzipSync(buf, { level: 9 }))
}

async function main() {
  if (process.env.PRERENDER === '0') return log('PRERENDER=0: skipped')
  const shellFile = path.join(DIST, 'index.html')
  if (!fs.existsSync(shellFile)) throw new Error('dist/index.html missing: run vite build first')
  const chrome = findChrome()
  if (!chrome) {
    log('WARNING: no Chrome/Chromium found (set CHROME_PATH): public routes are not prerendered')
    return
  }
  const { default: puppeteer } = await import('puppeteer-core')

  // Re-running on an already prerendered dist: start from the pristine shell.
  const shell = fs.existsSync(path.join(DIST, SHELL))
    ? fs.readFileSync(path.join(DIST, SHELL), 'utf8')
    : fs.readFileSync(shellFile, 'utf8')
  if (!shell.includes('<div id="root"></div>')) throw new Error('shell has no empty <div id="root"></div>')
  fs.writeFileSync(shellFile, shell)

  const port = await freePort()
  const base = `http://127.0.0.1:${port}`
  const vite = path.join(ROOT, 'node_modules', '.bin', 'vite')
  const server = spawn(
    vite,
    ['preview', '--mode', 'lighthouse', '--host', '127.0.0.1', '--port', String(port), '--strictPort'],
    {
      cwd: ROOT,
      stdio: ['ignore', 'pipe', 'pipe'],
      env: { ...process.env, GG_PRERENDER_SHELL_ONLY: '1' },
    },
  )
  let serverLog = ''
  server.stdout.on('data', (d) => (serverLog += d))
  server.stderr.on('data', (d) => (serverLog += d))

  const browser = await puppeteer.launch({
    executablePath: chrome,
    headless: true,
    // Container builds run as root, where Chrome refuses to start with its sandbox.
    args: [
      '--no-first-run',
      '--disable-extensions',
      '--lang=zh-CN',
      ...(process.getuid?.() === 0 ? ['--no-sandbox'] : []),
    ],
  })
  const out = {}
  try {
    await waitFor(`${base}/`)
    for (const [route, file] of Object.entries(ROUTES)) {
      const page = await browser.newPage()
      const problems = []
      page.on('pageerror', (e) => problems.push(String(e)))
      page.on('console', (m) => {
        if (m.type() === 'error') problems.push(m.text())
      })
      await page.setViewport({ width: 1280, height: 800 })
      await page.emulateMediaFeatures([{ name: 'prefers-color-scheme', value: 'light' }])
      await page.evaluateOnNewDocument(() => {
        localStorage.clear()
        window.__GG_PRERENDER__ = true
      })
      await page.goto(base + route, { waitUntil: 'domcontentloaded' })
      const snap = await page
        .waitForFunction(() => window.__GG_SNAPSHOT__, { timeout: 30000 })
        .then((h) => h.jsonValue())
      // Per-component HeroUI CSS arrives with JS chunks (scripts/heroui-css.ts): link what the first commit
      // used so the snapshot paints styled (no shift when the chunk CSS lands), and modulepreload the
      // route's JS so it downloads in parallel with the entry.
      const assets = await page.evaluate((at) => {
        const local = (u) => new URL(u, location.href).pathname
        const loaded = performance.getEntriesByType('resource').filter((e) => e.startTime <= at)
        return {
          css: loaded.map((e) => local(e.name)).filter((p) => p.startsWith('/assets/') && p.endsWith('.css')),
          js: loaded.map((e) => local(e.name)).filter((p) => p.startsWith('/assets/') && p.endsWith('.js')),
        }
      }, snap.at)
      snap.css = assets.css
      snap.js = assets.js
      await page.close()
      if (problems.length) throw new Error(`${route}: ${problems.join(' | ')}`)
      if (!snap.html.includes('<main') && !snap.html.includes('<form'))
        throw new Error(`${route}: unexpected snapshot`)
      out[route] = snap
      log(`${route} → dist/${file} (${(snap.html.length / 1024).toFixed(1)} KB, "${snap.title}")`)
    }
  } catch (error) {
    if (serverLog) console.error(serverLog)
    throw error
  } finally {
    await browser.close()
    server.kill('SIGTERM')
  }

  writeCompressed(path.join(DIST, SHELL), shell)

  // The shell's entry <script> and its modulepreloads start ~110 KB of JS before the first paint. On a
  // snapshot page that JS is not needed for the paint, so a tiny boot module starts it after the snapshot's
  // first paint has been reported (Lighthouse/Lantern otherwise counts every request started before FCP, and
  // React replacing the nodes before Chrome reports them moves LCP to the swap).
  const entry = shell.match(/<script type="module" crossorigin src="([^"]+)"><\/script>/)?.[1]
  if (!entry) throw new Error('shell has no module entry script')
  const shellPreloads = [...shell.matchAll(/<link rel="modulepreload" crossorigin href="([^"]+)">/g)].map(
    (m) => m[1],
  )
  const bootSrc = `// Generated by web/scripts/prerender.mjs: starts the app once the prerendered snapshot is painted.
const preload = (document.querySelector('meta[name="gg-preload"]')?.content || '').split(' ').filter(Boolean)
let started = false
const start = () => {
  if (started) return
  started = true
  for (const href of preload) {
    const link = document.createElement('link')
    link.rel = 'modulepreload'
    link.crossOrigin = ''
    link.href = href
    document.head.append(link)
  }
  import(${JSON.stringify(entry)})
}
// Chrome: the first LCP entry = the snapshot paint has been reported. Safari (no LCP): shortly after FCP.
try {
  new PerformanceObserver((_, observer) => {
    observer.disconnect()
    setTimeout(start, 0)
  }).observe({ type: 'largest-contentful-paint', buffered: true })
} catch {}
try {
  new PerformanceObserver((list, observer) => {
    if (!list.getEntriesByName('first-contentful-paint').length) return
    observer.disconnect()
    setTimeout(start, 50)
  }).observe({ type: 'paint', buffered: true })
} catch {}
// Hidden tab or no paint timing: do not wait for a paint that may never come.
setTimeout(start, document.visibilityState === 'visible' ? 1500 : 0)
`
  const bootName = `assets/boot-${createHash('sha256').update(bootSrc).digest('base64url').slice(0, 8)}.js`
  writeCompressed(path.join(DIST, bootName), bootSrc)

  for (const [route, file] of Object.entries(ROUTES)) {
    const { html, title, css, js } = out[route]
    const preload = [...new Set([entry, ...shellPreloads, ...js])].filter((h) => h !== entry)
    const head = [
      ...css
        .filter((h) => !shell.includes(`href="${h}"`))
        .map((h) => `<link rel="stylesheet" crossorigin href="${h}">`),
      `<meta name="gg-preload" content="${preload.join(' ')}">`,
      `<script type="module" crossorigin src="/${bootName}"></script>`,
    ].join('\n    ')
    // Inert snapshot forms: before the app takes over, Enter or a click must never submit natively (a GET
    // would put the password into the URL). Same markup otherwise, so the swap does not shift anything.
    const inert = html
      .replace(/<form(?![^>]*\bmethod=)/g, '<form method="post"')
      .replace(/<button type="submit"(?![^>]*\bdisabled)/g, '<button type="submit" disabled=""')
    const page = shell
      .replace(/\s*<script type="module" crossorigin src="[^"]+"><\/script>/, '')
      .replace(/\s*<link rel="modulepreload" crossorigin href="[^"]+">/g, '')
      .replace('<div id="root"></div>', `<div id="root">${inert}</div>`)
      .replace(/<title>[^<]*<\/title>/, `<title>${escapeHtml(title)}</title>`)
      .replace('  </head>', `    ${head}\n  </head>`)
    writeCompressed(path.join(DIST, file), page)
  }
}

main().catch((error) => {
  console.error(`[prerender] failed: ${error instanceof Error ? error.stack : error}`)
  process.exit(1)
})
