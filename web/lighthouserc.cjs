/**
 * Lighthouse CI — `pnpm lh` (builds first, then runs `lhci autorun` with this file).
 *
 * Serves `dist/` through `vite preview --mode lighthouse` (scripts/preview.ts): precompressed
 * brotli assets + immutable caching like the Go server, and fixture answers for the public API
 * (`/api/v1/guest/*`) so the landing page renders real plans without a backend.
 *
 *   LH_PRESET=mobile pnpm lh     # Lighthouse default mobile emulation (slow 4G, 4x CPU)
 *   LH_PRESET=desktop pnpm lh    # desktop preset (default)
 *   CHROME_PATH=/path/to/chrome pnpm lh
 */
const fs = require('node:fs')

const PORT = 4173
const base = `http://localhost:${PORT}`
const preset = process.env.LH_PRESET === 'mobile' ? 'mobile' : 'desktop'
const macChrome = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
const chromePath = process.env.CHROME_PATH || (fs.existsSync(macChrome) ? macChrome : undefined)

module.exports = {
  ci: {
    collect: {
      startServerCommand: `pnpm exec vite preview --mode lighthouse --port ${PORT} --strictPort`,
      startServerReadyPattern: 'Local',
      startServerReadyTimeout: 30000,
      url: [`${base}/`, `${base}/login`, `${base}/register`],
      numberOfRuns: 3,
      ...(chromePath ? { chromePath } : {}),
      settings: {
        ...(preset === 'desktop' ? { preset: 'desktop' } : {}),
        chromeFlags: '--headless=new --no-first-run --disable-extensions',
        // PWA category was removed in Lighthouse 12; audit the four scored categories.
        onlyCategories: ['performance', 'accessibility', 'best-practices', 'seo'],
      },
    },
    assert: {
      assertions: {
        'categories:performance': ['error', { minScore: 1, aggregationMethod: 'median-run' }],
        'categories:accessibility': ['error', { minScore: 1 }],
        'categories:best-practices': ['error', { minScore: 1 }],
        'categories:seo': ['error', { minScore: 1 }],
      },
    },
    upload: {
      // Reports stay local (no third-party upload): web/.lighthouseci/reports/*.html|json
      target: 'filesystem',
      outputDir: './.lighthouseci/reports',
      reportFilenamePattern: '%%HOSTNAME%%%%PATHNAME%%-%%DATETIME%%-report.%%EXTENSION%%',
    },
  },
}
