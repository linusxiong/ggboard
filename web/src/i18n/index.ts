/**
 * Tiny i18n: JSON dictionaries per locale/namespace, lazy-loaded with `import.meta.glob`.
 *
 *   src/i18n/<locale>/<namespace>.json   (nested objects allowed; keys are dot paths)
 *
 *   const t = useT('auth')                 // suspends until the namespace is loaded
 *   t('login.title')                        // → "登录"
 *   t('cooldown', { seconds: 30 })          // "{seconds} 秒后重试" → "30 秒后重试"
 *   t('common:actions.save')                // other namespace (must be loaded: pass ['auth','common'])
 *
 * Plurals: if `params.count` is a number and `<key>_<category>` exists (`_one`, `_other`, …), it wins.
 * Missing keys fall back to `en` (when loaded), then to the key itself.
 * Route loaders can preload namespaces: `loader: () => loadNamespaces(['shop'])`.
 */
import { use, useMemo } from 'react'
import type { Locale } from '@/lib/api/types'
import { DEFAULT_LOCALE, useUiStore } from '@/stores/ui'

export type Dict = { [key: string]: string | Dict }
export type TParams = Record<string, string | number>
export type TFunction = (key: string, params?: TParams) => string

export const FALLBACK_LOCALE: Locale = 'en'

const loaders = import.meta.glob<Dict>('./*/*.json', { import: 'default' })

const dicts = new Map<string, Dict>()
const pending = new Map<string, Promise<void>>()
const id = (locale: Locale, ns: string) => `${locale}/${ns}`

export function availableNamespaces(locale: Locale): string[] {
  const prefix = `./${locale}/`
  return Object.keys(loaders)
    .filter((k) => k.startsWith(prefix))
    .map((k) => k.slice(prefix.length, -'.json'.length))
}

export function isNamespaceLoaded(ns: string, locale: Locale = currentLocale()): boolean {
  return dicts.has(id(locale, ns))
}

/** Loads (once) `<locale>/<ns>.json`. Unknown namespaces resolve to an empty dictionary. */
export function loadNamespace(ns: string, locale: Locale = currentLocale()): Promise<void> {
  const key = id(locale, ns)
  if (dicts.has(key)) return Promise.resolve()
  const inflight = pending.get(key)
  if (inflight) return inflight
  const loader = loaders[`./${locale}/${ns}.json`]
  if (!loader) {
    if (import.meta.env.DEV) console.warn(`[i18n] missing namespace ${key}`)
    dicts.set(key, {})
    return Promise.resolve()
  }
  const promise = loader().then(
    (dict) => {
      dicts.set(key, dict)
      pending.delete(key)
    },
    (error: unknown) => {
      pending.delete(key)
      throw error
    },
  )
  pending.set(key, promise)
  return promise
}

const combined = new Map<string, Promise<void>>()

/**
 * Loads several namespaces. The returned promise is cached per (locale, list) so it is stable
 * across renders (required by React `use`).
 */
export function loadNamespaces(namespaces: readonly string[], locale: Locale = currentLocale()): Promise<void> {
  const key = `${locale}:${namespaces.join(',')}`
  const cached = combined.get(key)
  if (cached) return cached
  const promise = Promise.all(namespaces.map((ns) => loadNamespace(ns, locale))).then(
    () => {
      combined.delete(key)
    },
    (error: unknown) => {
      combined.delete(key)
      throw error
    },
  )
  combined.set(key, promise)
  return promise
}

/** Registers a dictionary synchronously (tests, SSR, or bundling a namespace eagerly). */
export function registerNamespace(ns: string, locale: Locale, dict: Dict) {
  dicts.set(id(locale, ns), dict)
}

export function currentLocale(): Locale {
  return useUiStore.getState().locale ?? DEFAULT_LOCALE
}

function lookup(dict: Dict | undefined, path: string): string | undefined {
  if (!dict) return undefined
  if (typeof dict[path] === 'string') return dict[path]
  let node: string | Dict | undefined = dict
  for (const part of path.split('.')) {
    if (typeof node !== 'object' || node === null) return undefined
    node = node[part]
  }
  return typeof node === 'string' ? node : undefined
}

const pluralRules = new Map<Locale, Intl.PluralRules>()
function pluralCategory(locale: Locale, count: number) {
  let rules = pluralRules.get(locale)
  if (!rules) {
    rules = new Intl.PluralRules(locale)
    pluralRules.set(locale, rules)
  }
  return rules.select(count)
}

export function interpolate(template: string, params?: TParams): string {
  if (!params) return template
  return template.replace(/\{(\w+)\}/g, (match, name: string) =>
    name in params ? String(params[name]) : match,
  )
}

function resolve(locale: Locale, ns: string, key: string, params?: TParams): string | undefined {
  const dict = dicts.get(id(locale, ns))
  if (typeof params?.count === 'number') {
    const plural = lookup(dict, `${key}_${pluralCategory(locale, params.count)}`)
    if (plural !== undefined) return plural
  }
  return lookup(dict, key)
}

/**
 * Translate `key` in namespace `ns` (or `other:key`). Pure function — usable outside React
 * (toasts, document.title) as long as the namespace is loaded.
 */
export function t(ns: string, key: string, params?: TParams, locale: Locale = currentLocale()): string {
  let namespace = ns
  let path = key
  const sep = key.indexOf(':')
  if (sep > 0) {
    namespace = key.slice(0, sep)
    path = key.slice(sep + 1)
  }
  const value =
    resolve(locale, namespace, path, params) ??
    (locale !== FALLBACK_LOCALE ? resolve(FALLBACK_LOCALE, namespace, path, params) : undefined)
  if (value === undefined) {
    if (import.meta.env.DEV) console.warn(`[i18n] missing key ${locale}/${namespace}:${path}`)
    return path
  }
  return interpolate(value, params)
}

/** Current locale (re-renders on change). */
export function useLocale(): Locale {
  return useUiStore((s) => s.locale)
}

/**
 * Returns a translator bound to the first namespace; suspends (React `use`) until every
 * listed namespace is loaded for the current locale. Wrap routes in `<Suspense>` (the root does).
 */
export function useT(namespaces: string | readonly string[]): TFunction {
  const locale = useLocale()
  const list = typeof namespaces === 'string' ? [namespaces] : namespaces
  const missing = list.filter((ns) => !dicts.has(id(locale, ns)))
  if (missing.length > 0) use(loadNamespaces(missing, locale))
  const primary = list[0] ?? 'common'
  const key = list.join('|')
  // eslint-disable-next-line react-hooks/exhaustive-deps -- `key` captures the namespace list
  return useMemo<TFunction>(() => (k, params) => t(primary, k, params, locale), [primary, locale, key])
}

/** Keeps `<html lang>` in sync with the store. Call once at startup. */
export function syncDocumentLanguage() {
  const apply = (locale: Locale) => {
    document.documentElement.lang = locale
  }
  apply(currentLocale())
  return useUiStore.subscribe((s, prev) => {
    if (s.locale !== prev.locale) apply(s.locale)
  })
}
