import { useVirtualizer } from '@tanstack/react-virtual'
import { Button } from '@heroui/react'
import { useQuery } from '@tanstack/react-query'
import { ChevronRight, RefreshCw } from 'lucide-react'
import { useCallback, useMemo, useState, useRef } from 'react'
import { TitleWithHelp } from '@/components/HelpTip'
import { SegmentedControl } from '@/components/SegmentedControl'
import { DebouncedSearch } from '@/features/admin/orders/DebouncedSearch'
import { SelectInput, SwitchInput } from '@/features/admin/settings/inputs/Controlled'
import { useT } from '@/i18n'
import { adminApi } from '@/lib/api'
import { qk } from '@/lib/api/queries'
import { cx } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'
import { formatLogTime } from './format'
import { filterLogs, LOG_LEVELS, LOG_LIMITS, type LogLevelFilter, logsQuery } from './logs'

/** Rows above this count are windowed with TanStack Virtual. */
const VIRTUAL_THRESHOLD = 60

const LEVEL_CLASS: Record<string, string> = {
  debug: 'text-label-secondary',
  info: 'text-label-blue',
  warn: 'text-label-orange',
  warning: 'text-label-orange',
  error: 'text-label-red',
}

export function LogsViewer() {
  const t = useT(['admin-system', 'common'])
  const [level, setLevel] = useState<LogLevelFilter>('all')
  const [limit, setLimit] = useState<number>(200)
  const [auto, setAuto] = useState(false)
  const [text, setText] = useState<string>()
  const [open, setOpen] = useState<ReadonlySet<string>>(new Set())
  const query = logsQuery(level, limit)
  const logs = useQuery({
    queryKey: qk.admin.logs(query),
    queryFn: ({ signal }) => adminApi.systemLogEntries(query, signal),
    refetchInterval: auto ? 10_000 : false,
  })
  // Newest first (the ring buffer is returned oldest → newest).
  const rows = useMemo(() => {
    const list = filterLogs(logs.data ?? [], text)
    return list.sort((a, b) => Date.parse(b.time) - Date.parse(a.time))
  }, [logs.data, text])
  const onSearch = useCallback((v: string | undefined) => setText(v), [])
  const scrollRef = useRef<HTMLDivElement>(null)
  // Long log lists (up to 1000 rows) only render the visible window; rows measure themselves because
  // expanded attrs change their height.
  // Only long lists are virtualized; short ones keep the plain layout (also what jsdom tests see).
  const virtual = rows.length > VIRTUAL_THRESHOLD
  // eslint-disable-next-line react-hooks/incompatible-library -- TanStack Virtual returns non-memoizable functions by design
  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => 36,
    overscan: 12,
    initialRect: { width: 960, height: 576 },
  })

  const toggle = (id: string) =>
    setOpen((s) => {
      const next = new Set(s)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  return (
    <section aria-labelledby="logs-title" className="surface-card flex flex-col">
      <div className="flex flex-col gap-4 p-5 hairline-b">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <TitleWithHelp help={t('logs.help')} subject={t('logs.title')}>
            <h2 id="logs-title" className="text-title3 text-label">
              {t('logs.title')}
            </h2>
          </TitleWithHelp>
          <Button
            size="sm"
            variant="secondary"
            isPending={logs.isFetching}
            onPress={() => void logs.refetch()}
          >
            <RefreshCw className="size-4" aria-hidden />
            {t('common:actions.refresh')}
          </Button>
        </div>
        <div className="flex flex-wrap items-end gap-4">
          <SegmentedControl
            label={t('logs.level')}
            value={level}
            onChange={setLevel}
            items={LOG_LEVELS.map((l) => ({ id: l, label: t(`logs.levels.${l}`) }))}
          />
          <SelectInput
            label={t('logs.limit')}
            name="log_limit"
            value={String(limit)}
            onChange={(v) => v && setLimit(Number(v))}
            options={LOG_LIMITS.map((n) => ({ value: String(n), label: String(n) }))}
            className="w-28"
          />
          <DebouncedSearch
            label={t('logs.search')}
            value={text}
            onCommit={onSearch}
            placeholder={t('logs.searchPlaceholder')}
            delay={150}
            className="min-w-48 flex-1"
          />
          <SwitchInput label={t('logs.auto')} name="log_auto" value={auto} onChange={setAuto} />
        </div>
      </div>

      <div ref={scrollRef} className="max-h-[36rem] overflow-auto" aria-busy={logs.isFetching}>
        {logs.isError ? (
          <p className="p-5 text-subhead text-label-red">{errorMessage(logs.error)}</p>
        ) : logs.isPending ? (
          <p className="p-5 text-subhead text-label-secondary">{t('common:states.loading')}</p>
        ) : rows.length === 0 ? (
          <p className="p-5 text-subhead text-label-secondary">{t('logs.empty')}</p>
        ) : (
          <ol
            aria-label={t('logs.title')}
            className={cx('font-mono text-footnote', virtual && 'relative')}
            style={virtual ? { height: virtualizer.getTotalSize() } : undefined}
          >
            {(virtual
              ? virtualizer.getVirtualItems().map((item) => ({ i: item.index, start: item.start }))
              : rows.map((_, i) => ({ i, start: 0 }))
            ).map(({ i, start }) => {
              const e = rows[i]!
              const attrs = e.attrs && Object.keys(e.attrs).length ? e.attrs : null
              const id = `${e.time}|${e.msg}|${i}`
              const rowKey = `${e.time}|${e.msg}`
              const expanded = open.has(rowKey)
              return (
                <li
                  key={id}
                  ref={virtual ? virtualizer.measureElement : undefined}
                  data-index={i}
                  className={cx('hairline-b px-5 py-1.5', virtual && 'absolute inset-x-0 top-0')}
                  style={virtual ? { transform: `translateY(${start}px)` } : undefined}
                >
                  <div className="flex items-start gap-3">
                    <time dateTime={e.time} className="shrink-0 text-label-secondary tabular">
                      {formatLogTime(e.time)}
                    </time>
                    <span
                      className={cx(
                        'w-12 shrink-0 font-semibold uppercase',
                        LEVEL_CLASS[e.level.toLowerCase()] ?? 'text-label',
                      )}
                    >
                      {e.level}
                    </span>
                    <span className="min-w-0 flex-1 break-words whitespace-pre-wrap text-label">{e.msg}</span>
                    {attrs ? (
                      <button
                        type="button"
                        aria-expanded={expanded}
                        onClick={() => toggle(rowKey)}
                        className="inline-flex shrink-0 items-center gap-0.5 rounded-[6px] px-1 text-label-blue outline-none focus-visible:ring-2 focus-visible:ring-focus"
                      >
                        <ChevronRight
                          className={cx('size-3.5 transition-transform', expanded && 'rotate-90')}
                          aria-hidden
                        />
                        {t('logs.attrs', { n: Object.keys(attrs).length })}
                      </button>
                    ) : null}
                  </div>
                  {attrs && expanded ? (
                    <pre className="mt-1 ms-4 overflow-x-auto rounded-[8px] bg-fill-quaternary p-2 text-caption text-label">
                      {JSON.stringify(attrs, null, 2)}
                    </pre>
                  ) : null}
                </li>
              )
            })}
          </ol>
        )}
      </div>
      <p className="px-5 py-2 text-caption text-label-secondary">
        {t('logs.footer', { shown: rows.length, total: logs.data?.length ?? 0 })}
      </p>
    </section>
  )
}
