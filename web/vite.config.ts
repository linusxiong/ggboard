/// <reference types="vitest/config" />
import { fileURLToPath } from 'node:url'
import tailwindcss from '@tailwindcss/vite'
import { tanstackRouter } from '@tanstack/router-plugin/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'
import { compression } from 'vite-plugin-compression2'
import { herouiComponentCss } from './scripts/heroui-css.ts'
import { productionPreview } from './scripts/preview.ts'

const API_TARGET = process.env.GGBOARD_API ?? 'http://localhost:8080'

/** Vendor groups — long-lived chunks that change rarely and cache well. */
const vendorGroups = [
  { name: 'react', test: /node_modules[\\/](react|react-dom|scheduler)[\\/]/, priority: 30 },
  {
    name: 'tanstack',
    test: /node_modules[\\/]@tanstack[\\/](react-router|router-core|history|react-store|store)[\\/]/,
    priority: 20,
  },
  {
    // React Query is only loaded by the authenticated shells (see src/lib/queryClient.ts).
    name: 'tanstack-query',
    test: /node_modules[\\/]@tanstack[\\/](react-query|query-core)[\\/]/,
    priority: 20,
  },
  {
    // App core that every page needs (entry-adjacent): one file compresses better than five.
    name: 'app',
    test: /[\\/]src[\\/](i18n[\\/]index|stores[\\/]|lib[\\/](api[\\/](client|guest|passport|types)|publicData|theme|motion|toast|guards|cn|errors)\.)/,
    priority: 15,
  },
  {
    // Query factories: dynamically imported by route loaders (entry) AND statically by lazy chunks, so
    // without a group Rolldown hoists them into the entry chunk — and React Query with them (+10 KB gz on
    // the public pages). Lower priority than `app` so shared API modules stay where they were.
    name: 'queries',
    test: /[\\/]src[\\/]lib[\\/]api[\\/]queries\./,
    priority: 12,
  },
  {
    // Chrome shared by every page (native HeroUI-styled buttons, toggles, logo, inline icons).
    name: 'chrome',
    test: /[\\/]src[\\/](components[\\/](native[\\/]NativeButton|icons[\\/]|ThemeToggle|LocaleToggle|Logo)|lib[\\/]button)/,
    priority: 14,
  },
  {
    // Auth pages kit (login / register / forgot share it).
    name: 'auth-kit',
    test: /[\\/]src[\\/](components[\\/](native[\\/]NativeField|auth[\\/]|layout[\\/]AuthLayout|FormError|LazyCaptcha)|hooks[\\/](useLiteForm|useFinishAuth|useCooldown)|lib[\\/](validators|authErrors))/,
    priority: 13,
  },
  {
    // HeroUI + React Aria are big; `entriesAware` keeps per-route subsets so the
    // login/landing routes never download Table/DatePicker/etc.
    name: 'heroui',
    test: /node_modules[\\/](@heroui|react-aria|react-aria-components|react-stately|@react-aria|@react-stately|@react-types|@internationalized|tailwind-variants|tailwind-merge|@swc[\\/]helpers|clsx)[\\/]/,
    priority: 10,
    entriesAware: true,
  },
]

export default defineConfig({
  plugins: [
    tanstackRouter({
      target: 'react',
      autoCodeSplitting: true,
      routesDirectory: './src/routes',
      generatedRouteTree: './src/routeTree.gen.ts',
      quoteStyle: 'single',
      semicolons: false,
    }),
    react(),
    herouiComponentCss(),
    tailwindcss(),
    compression({
      algorithms: ['brotliCompress', 'gzip'],
      include: /\.(js|mjs|css|html|svg|json|webmanifest|txt|xml)$/i,
      threshold: 512,
    }),
    productionPreview(),
  ],
  resolve: {
    tsconfigPaths: true,
    alias: [
      // React 19 has useSyncExternalStore built in; skip the npm shim TanStack Store pulls in.
      {
        find: /^use-sync-external-store\/shim(\/index\.js)?$/,
        replacement: fileURLToPath(new URL('./src/shims/use-sync-external-store.ts', import.meta.url)),
      },
    ],
  },
  server: {
    proxy: {
      '/api': { target: API_TARGET, changeOrigin: true },
      '/s/': { target: API_TARGET, changeOrigin: true },
    },
  },
  preview: {
    proxy: {
      '/api': { target: API_TARGET, changeOrigin: true },
    },
  },
  build: {
    // Default target = Vite's baseline-widely-available (Chrome/Edge 111, Firefox 114, Safari 16.4) —
    // the same floor as Tailwind v4, so lightningcss emits no lab()/color-mix() fallbacks.
    modulePreload: { polyfill: false },
    cssMinify: 'lightningcss',
    chunkSizeWarningLimit: 400,
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: vendorGroups,
        },
      },
    },
  },
  test: {
    environment: 'jsdom',
    globals: false,
    css: false,
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
    restoreMocks: true,
  },
})
