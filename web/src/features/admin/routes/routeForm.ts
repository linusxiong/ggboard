/**
 * Node route form model ↔ API (`{remarks, match[], action, action_value}`) and client-side
 * checks mirroring docs/NODE_PROTOCOLS.md §2.1.9 / §8: at most 999 match entries and, for
 * `block` routes, the `|`-joined match must compile as a Go (RE2) regexp — XrayR panics otherwise.
 */
import type { NodeRoute, NodeRouteAction, NodeRouteInput } from '@/lib/api/types'

export interface RouteFormValues {
  remarks: string
  /** One match entry per line. */
  match: string
  action: NodeRouteAction
  action_value: string
}

export const MAX_MATCH = 999

export function routeToValues(route?: NodeRoute | null): RouteFormValues {
  return {
    remarks: route?.remarks ?? '',
    match: (route?.match ?? []).join('\n'),
    action: route?.action ?? 'block',
    action_value: route?.action_value ?? '',
  }
}

const QUOTED = `(?:"[^"]*"|'[^']*')`
/** A line made only of quoted items separated by commas: `"a",` · `'a', 'b'` · `"a"`. */
const QUOTED_RUN = new RegExp(`^${QUOTED}(?:\\s*[,，]\\s*${QUOTED})*\\s*[,，]?$`)

/** Curly quotes pasted from chat apps or documents → straight ones (only used to recognise quoted items). */
const straighten = (text: string) => text.replace(/[“”„]/g, '"').replace(/[‘’]/g, "'")

const splitLines = (text: string) =>
  text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)

/** Raw DNS JSON route (`main` on the first line): quotes, commas and repeated lines are significant there. */
const isRawDns = (lines: string[], action: NodeRouteAction) => action === 'dns' && lines[0] === 'main'

/**
 * Match entries from a textarea, one per line, tolerant of pasted lists: a line may be wrapped in
 * quotes (`"geosite:bbc",`, `'domain:rfa.org'`) and end with a comma, several quoted items may share
 * a line, and the whole list may sit between `[` and `]` (a JSON array). Quotes and commas are
 * dropped, blanks and repeats removed. Mirrors `CleanMatch` in internal/service/node/validate.go.
 */
export function parseMatchList(text: string, action: NodeRouteAction): string[] {
  let lines = splitLines(text)
  if (isRawDns(lines, action)) return lines
  const body = text.trim()
  if (body.startsWith('[') && body.endsWith(']')) {
    const inner = splitLines(body.slice(1, -1))
    if (inner.every((line) => QUOTED_RUN.test(straighten(line)))) lines = inner
  }
  const out = new Set<string>()
  for (const line of lines) {
    const flat = straighten(line)
    if (QUOTED_RUN.test(flat)) {
      for (const m of flat.matchAll(/"([^"]*)"|'([^']*)'/g)) {
        const item = (m[1] ?? m[2] ?? '').trim()
        if (item) out.add(item)
      }
    } else {
      const item = line.replace(/[\s,，]+$/, '')
      if (item) out.add(item)
    }
  }
  return [...out]
}

/** Prefixes only soga understands in a block rule; XrayR and V2bX match them as plain regexps (docs/NODE_PROTOCOLS.md §3.3). */
const SOGA_ONLY_PREFIXES = ['geosite:', 'geoip:', 'domain:', 'full:', 'ip:', 'port:'] as const

/** The soga-only prefixes used by the entries of a block rule, in a fixed order (empty = none). */
export function sogaOnlyPrefixes(entries: readonly string[]): string[] {
  const lower = entries.map((entry) => entry.toLowerCase())
  return SOGA_ONLY_PREFIXES.filter((prefix) => lower.some((entry) => entry.startsWith(prefix)))
}

/** The cleaned text a pasted list turns into (shown when the field loses focus). */
export function normalizeMatchText(text: string, action: NodeRouteAction): string {
  return parseMatchList(text, action).join('\n')
}

export function valuesToRouteInput(values: RouteFormValues): NodeRouteInput {
  return {
    remarks: values.remarks.trim(),
    match: parseMatchList(values.match, values.action),
    action: values.action,
    action_value: values.action_value.trim(),
  }
}

/**
 * Approximates Go's RE2 parser with the JS engine: Go-only syntax is translated first, and
 * constructs RE2 rejects (look-around, back-references) are reported explicitly.
 * Returns an error description, or `null` when the pattern is acceptable.
 */
export function goRegexpError(pattern: string): string | null {
  if (/\(\?<?[=!]/.test(pattern)) return 'lookaround'
  if (/\\[1-9]/.test(pattern)) return 'backreference'
  const js = pattern
    .replace(/\(\?P<(\w+)>/g, '(?<$1>')
    .replace(/\(\?[imsU]+\)/g, '')
    .replace(/\(\?[imsU]+(-[imsU]+)?:/g, '(?:')
  try {
    new RegExp(js)
    return null
  } catch (error) {
    return error instanceof Error ? error.message : 'invalid'
  }
}

export type MatchIssue =
  { key: 'required' } | { key: 'tooMany'; max: number } | { key: 'regexp'; detail: string }

export function checkMatch(text: string, action: NodeRouteAction): MatchIssue | undefined {
  const lines = parseMatchList(text, action)
  if (lines.length === 0) return { key: 'required' }
  if (lines.length > MAX_MATCH) return { key: 'tooMany', max: MAX_MATCH }
  if (action === 'block') {
    const detail = goRegexpError(lines.join('|'))
    if (detail) return { key: 'regexp', detail }
  }
  return undefined
}
