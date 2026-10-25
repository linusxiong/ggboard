/** Small node table pieces: status badge (with last check/push tooltip), load mini-bars, show/hide toggle. */
import { Button, Tooltip } from '@heroui/react'
import { Eye, EyeOff } from 'lucide-react'
import { useT } from '@/i18n'
import type { NodeLoadValue, NodeStatus } from '@/lib/api/types'
import { cx } from '@/lib/cn'
import { formatPercent } from '@/lib/format'
import type { Formatters } from '../shared/useFormatters'
import { useNow } from '../shared/useNow'
import { loadRatio, nodeState } from './status'

const DOT = { online: 'bg-system-green', idle: 'bg-system-orange', offline: 'bg-system-gray' } as const

export function StatusBadge({ status, fmt }: { status: NodeStatus | null | undefined; fmt: Formatters }) {
  const t = useT('admin-nodes')
  const now = useNow()
  const state = nodeState(status, now)
  const detail = [
    t('nodes.status.lastCheck', { time: status?.last_check_at ? fmt.dateTime(status.last_check_at) : '—' }),
    t('nodes.status.lastPush', { time: status?.last_push_at ? fmt.dateTime(status.last_push_at) : '—' }),
    ...(status?.version ? [t('nodes.status.version', { version: status.version })] : []),
  ]
  return (
    <Tooltip delay={200}>
      <Tooltip.Trigger className="inline-flex items-center gap-1.5 rounded-[8px] text-subhead text-label outline-offset-2">
        <span className={cx('size-2 shrink-0 rounded-full', DOT[state])} aria-hidden="true" />
        {t(`nodes.status.${state}`)}
        <span className="sr-only">{`: ${detail.join('; ')}`}</span>
      </Tooltip.Trigger>
      <Tooltip.Content>
        <ul className="flex flex-col gap-0.5 text-footnote">
          {detail.map((line) => (
            <li key={line}>{line}</li>
          ))}
        </ul>
      </Tooltip.Content>
    </Tooltip>
  )
}

function Bar({ label, ratio, locale }: { label: string; ratio: number; locale: Formatters['locale'] }) {
  const tone = ratio >= 0.9 ? 'bg-system-red' : ratio >= 0.7 ? 'bg-system-orange' : 'bg-system-blue'
  const pct = formatPercent(ratio, locale)
  return (
    <span className="flex items-center gap-1.5">
      <span className="w-7 text-caption2 text-label-secondary">{label}</span>
      <span
        role="meter"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(ratio * 100)}
        aria-valuetext={pct}
        className="h-1.5 w-10 overflow-hidden rounded-full bg-fill-tertiary"
      >
        <span
          className={cx('block h-full rounded-full', tone)}
          style={{ width: `${Math.round(ratio * 100)}%` }}
        />
      </span>
      <span className="w-8 text-end text-caption2 tabular text-label-secondary" aria-hidden="true">
        {pct}
      </span>
    </span>
  )
}

/** CPU / memory / disk mini-bars from `status.load` (`—` when the backend reports none). */
export function LoadBars({
  load,
  fmt,
}: {
  load: Record<string, unknown> | null | undefined
  fmt: Formatters
}) {
  const t = useT('admin-nodes')
  if (!load) return <span className="text-label-secondary">—</span>
  const items = (['cpu', 'mem', 'disk'] as const)
    .map((key) => ({ key, ratio: loadRatio(load[key] as NodeLoadValue | undefined) }))
    .filter((i): i is { key: 'cpu' | 'mem' | 'disk'; ratio: number } => i.ratio !== null)
  if (items.length === 0) return <span className="text-label-secondary">—</span>
  return (
    <span className="flex flex-col gap-0.5">
      {items.map((i) => (
        <Bar key={i.key} label={t(`nodes.load.${i.key}`)} ratio={i.ratio} locale={fmt.locale} />
      ))}
    </span>
  )
}

/**
 * Icon-only show/hide toggle (`aria-pressed` = shown in user subscriptions). Hidden nodes get the
 * eye-off glyph in the secondary label colour; the tooltip names the current state.
 */
export function VisibilityToggle({
  name,
  shown,
  onChange,
}: {
  name: string
  shown: boolean
  onChange: (shown: boolean) => void
}) {
  const t = useT('admin-nodes')
  const Icon = shown ? Eye : EyeOff
  return (
    <Tooltip delay={200}>
      <Button
        size="sm"
        variant="ghost"
        isIconOnly
        aria-label={t('nodes.visibility.label', { name })}
        aria-pressed={shown}
        onPress={() => onChange(!shown)}
        className={shown ? undefined : 'text-label-secondary'}
      >
        <Icon className="size-4" aria-hidden="true" />
      </Button>
      <Tooltip.Content>{shown ? t('nodes.visibility.shown') : t('nodes.visibility.hidden')}</Tooltip.Content>
    </Tooltip>
  )
}
