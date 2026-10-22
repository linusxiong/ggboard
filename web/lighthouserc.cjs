/**
 * Lighthouse CI — `pnpm lh` builds (vite build + prerender) and asserts 100/100/100/100 on the public pages
 * with BOTH presets (`lh:desktop`, then `lh:mobile` = Lighthouse's default mobile emulation: slow 4G, 4x CPU).
 *
 * Server: by default `vite preview --mode lighthouse` (scripts/preview.ts) — served like the Go server
 * (prerendered dist/<route>/index.html, spa.html fallback, brotli + immutable assets) with fixture answers for
 * the public API. Point it at a real server instead with LH_BASE (e.g. the Go binary serving the embedded dist):
 *
 *   LH_PRESET=mobile pnpm exec lhci autorun --config=./lighthouserc.cjs
 *   LH_BASE=http://127.0.0.1:18090 LH_PRESET=desktop pnpm exec lhci autorun --config=./lighthouserc.cjs
 *   LH_BASE=… LH_TARGET=auth LH_USER=user@example.com LH_PASSWORD=… pnpm exec lhci autorun --config=./lighthouserc.cjs
 *     → signed-in pages (/dashboard, /subscription) through scripts/lh-login.cjs (needs a real backend)
 *   CHROME_PATH=/path/to/chrome …
 */
const fs = require('node:fs')

const PORT = 4173
const external = process.env.LH_BASE ? process.env.LH_BASE.replace(/\/+$/, '') : ''
const base = external || `http://localhost:${PORT}`
const preset = process.env.LH_PRESET === 'mobile' ? 'mobile' : 'desktop'
const auth = process.env.LH_TARGET === 'auth'
const macChrome = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
const chromePath = process.env.CHROME_PATH || (fs.existsSync(macChrome) ? macChrome : undefined)

if (auth && !external) throw new Error('LH_TARGET=auth needs a real backend: set LH_BASE')

const pages = auth ? ['/dashboard', '/subscription'] : ['/', '/login', '/register', '/forgot']

module.exports = {
  ci: {
    collect: {
      ...(external
        ? {}
        : {
            startServerCommand: `pnpm exec vite preview --mode lighthouse --port ${PORT} --strictPort`,
            startServerReadyPattern: 'Local',
            startServerReadyTimeout: 30000,
          }),
      url: pages.map((p) => `${base}${p}`),
      numberOfRuns: 3,
      ...(chromePath ? { chromePath } : {}),
      ...(auth ? { puppeteerScript: './scripts/lh-login.cjs' } : {}),
      settings: {
        ...(preset === 'desktop' ? { preset: 'desktop' } : {}),
        chromeFlags: '--headless=new --no-first-run --disable-extensions',
        // Keep the session the puppeteer script stored (Lighthouse clears origin storage by default).
        ...(auth ? { disableStorageReset: true } : {}),
        // PWA category was removed in Lighthouse 12; audit the four scored categories.
        onlyCategories: ['performance', 'accessibility', 'best-practices', 'seo'],
      },
    },
    assert: {
      assertions: {
        'categories:performance': ['error', { minScore: 1, aggregationMethod: 'median-run' }],
        'categories:accessibility': ['error', { minScore: 1 }],
        'categories:best-practices': ['error', { minScore: 1 }],
        // Signed-in pages are noindex-by-design candidates; SEO is asserted on the public pages only.
        'categories:seo': auth ? 'off' : ['error', { minScore: 1 }],
      },
    },
    upload: {
      // Reports stay local (no third-party upload): web/.lighthouseci/reports/*.html|json
      target: 'filesystem',
      outputDir: './.lighthouseci/reports',
      reportFilenamePattern: `%%HOSTNAME%%%%PATHNAME%%-${preset}-%%DATETIME%%-report.%%EXTENSION%%`,
    },
  },
}
