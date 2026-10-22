/**
 * Inline copies of a few lucide icons (ISC license, lucide-react v1.60) for the PUBLIC pages
 * (landing / auth / theme & locale toggles). They avoid shipping lucide's runtime in the
 * first-load bundle (AGENTS.md §6: < 120 KB gzip). Everywhere else import from `lucide-react`.
 * Generated from node_modules/lucide-react/dist/esm/icons/*.mjs — same API: className, aria-hidden.
 */
import type { SVGProps } from 'react'

type IconNode = ReadonlyArray<readonly [string, Record<string, string>]>
export type InlineIconProps = SVGProps<SVGSVGElement>

export function make(node: IconNode, name: string) {
  function Icon(props: InlineIconProps) {
    return (
      <svg
        xmlns="http://www.w3.org/2000/svg"
        width="24"
        height="24"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
        focusable="false"
        {...props}
      >
        {node.map(([tag, attrs], i) => {
          const Tag = tag as 'path'
          return <Tag key={i} {...attrs} />
        })}
      </svg>
    )
  }
  Icon.displayName = name
  return Icon
}

export const Monitor = /* @__PURE__ */ make(
  [
    ['rect', { width: '20', height: '14', x: '2', y: '3', rx: '2' }],
    ['line', { x1: '8', x2: '16', y1: '21', y2: '21' }],
    ['line', { x1: '12', x2: '12', y1: '17', y2: '21' }],
  ],
  'Monitor',
)

export const Sun = /* @__PURE__ */ make(
  [
    ['circle', { cx: '12', cy: '12', r: '4' }],
    ['path', { d: 'M12 2v2' }],
    ['path', { d: 'M12 20v2' }],
    ['path', { d: 'm4.93 4.93 1.41 1.41' }],
    ['path', { d: 'm17.66 17.66 1.41 1.41' }],
    ['path', { d: 'M2 12h2' }],
    ['path', { d: 'M20 12h2' }],
    ['path', { d: 'm6.34 17.66-1.41 1.41' }],
    ['path', { d: 'm19.07 4.93-1.41 1.41' }],
  ],
  'Sun',
)

export const Moon = /* @__PURE__ */ make(
  [
    [
      'path',
      {
        d: 'M20.985 12.486a9 9 0 1 1-9.473-9.472c.405-.022.617.46.402.803a6 6 0 0 0 8.268 8.268c.344-.215.825-.004.803.401',
      },
    ],
  ],
  'Moon',
)

export const Languages = /* @__PURE__ */ make(
  [
    ['path', { d: 'm5 8 6 6' }],
    ['path', { d: 'm4 14 6-6 2-3' }],
    ['path', { d: 'M2 5h12' }],
    ['path', { d: 'M7 2h1' }],
    ['path', { d: 'm22 22-5-10-5 10' }],
    ['path', { d: 'M14 18h6' }],
  ],
  'Languages',
)

export const UserX = /* @__PURE__ */ make(
  [
    ['path', { d: 'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2' }],
    ['circle', { cx: '9', cy: '7', r: '4' }],
    ['line', { x1: '17', x2: '22', y1: '8', y2: '13' }],
    ['line', { x1: '22', x2: '17', y1: '8', y2: '13' }],
  ],
  'UserX',
)

export const Inbox = /* @__PURE__ */ make(
  [
    ['polyline', { points: '22 12 16 12 14 15 10 15 8 12 2 12' }],
    [
      'path',
      {
        d: 'M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z',
      },
    ],
  ],
  'Inbox',
)
