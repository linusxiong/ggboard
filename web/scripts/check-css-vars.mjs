#!/usr/bin/env node
/**
 * Fails the build when the built CSS reads a custom property with a bare `var(--x)` that no stylesheet defines.
 *
 * Such a declaration is void: a `transition` or `animation` does not run and a size falls back to its initial
 * value, with no error anywhere. It happens here because every HeroUI component stylesheet is its own Tailwind
 * build (scripts/heroui-css.ts), and Tailwind writes a theme variable only into the build that uses it. The plugin
 * defines the theme variables each component reads; this check catches whatever a HeroUI update adds that the
 * plugin does not cover (MISTAKES #313).
 *
 *   node scripts/check-css-vars.mjs [dist]
 */
import fs from 'node:fs'
import path from 'node:path'

/** Set from JavaScript or an inline style at runtime, so no stylesheet defines them. */
const RUNTIME = new Set([
  // React Aria / HeroUI
  '--disclosure-panel-height',
  '--trigger-anchor-point',
  '--trigger-width',
  '--visual-viewport-height',
  // components/effects (inline styles)
  '--gg-color',
  '--gg-glow',
  '--gg-shine',
  // Read by HeroUI's input groups but defined nowhere in @heroui/styles 3.2.7, its own full stylesheet included.
  '--color-field-border-invalid',
])

const assets = path.join(process.argv[2] ?? 'dist', 'assets')
const sheets = fs
  .readdirSync(assets)
  .filter((file) => file.endsWith('.css'))
  .map((file) => [file, fs.readFileSync(path.join(assets, file), 'utf8')])
const all = sheets.map(([, css]) => css).join('\n')
const defined = new Set([
  ...[...all.matchAll(/(--[\w-]+)\s*:/g)].map((m) => m[1]),
  ...[...all.matchAll(/@property\s+(--[\w-]+)/g)].map((m) => m[1]),
])

const missing = new Map()
for (const [file, css] of sheets) {
  for (const [, name] of css.matchAll(/var\(\s*(--[\w-]+)\s*\)/g)) {
    if (defined.has(name) || RUNTIME.has(name)) continue
    missing.set(name, [...(missing.get(name) ?? []), file])
  }
}

if (missing.size) {
  console.error('[check-css-vars] read with a bare var() but defined in no stylesheet:')
  for (const [name, files] of [...missing].sort()) {
    console.error(`  ${name}  (${[...new Set(files)].slice(0, 3).join(', ')})`)
  }
  console.error(
    'A theme variable belongs in scripts/heroui-css.ts (see its header); one that is set at runtime goes on the RUNTIME list above.',
  )
  process.exit(1)
}
console.log(`[check-css-vars] ${sheets.length} stylesheets, every bare var() is defined`)
