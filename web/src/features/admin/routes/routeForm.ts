/**
 * Node route form model ↔ API (`{remarks, match[], action, action_value}`) and client-side
 * checks mirroring docs/NODE_PROTOCOLS.md §2.1.9 / §8: at most 999 match entries and, for
 * `block` routes, the `|`-joined match must compile as a Go (RE2) regexp — XrayR panics otherwise.
 */
import type { NodeRoute, NodeRouteAction, NodeRouteInput } from '@/lib/api/types'
import { parseLines } from '../shared/units'

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

export function valuesToRouteInput(values: RouteFormValues): NodeRouteInput {
  return {
    remarks: values.remarks.trim(),
    match: parseLines(values.match),
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
  const lines = parseLines(text)
  if (lines.length === 0) return { key: 'required' }
  if (lines.length > MAX_MATCH) return { key: 'tooMany', max: MAX_MATCH }
  if (action === 'block') {
    const detail = goRegexpError(lines.join('|'))
    if (detail) return { key: 'regexp', detail }
  }
  return undefined
}
