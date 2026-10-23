/**
 * iOS segmented control (UISegmentedControl) — the one control for filters, mode switches and content tabs.
 *
 *   <SegmentedControl aria-label="订单状态" value={status} onChange={setStatus}
 *     items={[{ id: 'all', label: '全部' }, { id: '0', label: '待支付', count: 2 }]} />
 *
 *   <SegmentedTabs value={tab} onChange={setTab}>            // content tabs (tablist + tabpanels)
 *     <SegmentedTabs.List aria-label="个人中心" items={[…]} />
 *     <SegmentedTabs.Panel id="account">…</SegmentedTabs.Panel>
 *   </SegmentedTabs>
 *
 * Look (app.css `.gg-segmented*`): hug-content grey track (`--segment-track`, 2 px inset, 9 px radius), 32 px
 * segments (28 px `size="sm"`) with 13 px semibold labels, one white (dark #636366) pill with a soft shadow that
 * slides to the selection, hairline dividers between unselected segments. On narrow screens the track scrolls
 * inside an edge-faded container and keeps the selection in view. Motion is CSS only (the global
 * reduced-motion rule turns the slide into a jump).
 *
 * Semantics: `SegmentedControl` = React Aria `ToggleButtonGroup` (radiogroup + radio) with radio keyboard
 * behaviour — one tab stop (the selected segment), ←/→/↑/↓ move AND select (wrapping), Home/End.
 * `SegmentedTabs` = React Aria `Tabs` (tablist + tab + tabpanel, automatic activation).
 */
import {
  type KeyboardEvent,
  type ReactNode,
  useContext,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
} from 'react'
import {
  Tab,
  TabList,
  TabListStateContext,
  TabPanel,
  type TabPanelProps,
  Tabs,
  ToggleButton,
  ToggleButtonGroup,
  useLocale,
} from 'react-aria-components'
import { cx } from '@/lib/cn'

export interface SegmentItem<K extends string = string> {
  id: K
  label: ReactNode
  /** Accessible name when `label` is not plain text. */
  textValue?: string
  /** Leading symbol (16 px). */
  icon?: ReactNode
  /** Trailing count capsule (`null`/`undefined` hides it; 0 is shown). */
  count?: number | null
  /** `accent` = tinted capsule for "needs attention" counts. */
  countTone?: 'default' | 'accent'
  /** Any other trailing adornment (status dot, …). */
  adornment?: ReactNode
  isDisabled?: boolean
}

type Size = 'sm' | 'md'

interface Labelling {
  'aria-label'?: string
  'aria-labelledby'?: string
}

/* ------------------------------------------------------------------ shared track (pill + fades) */

function useSegmentTrack(value: string | null, layoutKey: string) {
  const scrollerRef = useRef<HTMLDivElement>(null)
  const trackRef = useRef<HTMLDivElement>(null)
  const pillRef = useRef<HTMLSpanElement>(null)

  // Measure the selected segment and move the pill there; `reveal` scrolls it into view.
  useLayoutEffect(() => {
    const scroller = scrollerRef.current
    const track = trackRef.current
    const pill = pillRef.current
    if (!scroller || !track || !pill) return

    const fades = () => {
      const max = scroller.scrollWidth - scroller.clientWidth
      scroller.toggleAttribute('data-fade-start', max > 1 && scroller.scrollLeft > 1)
      scroller.toggleAttribute('data-fade-end', max > 1 && scroller.scrollLeft < max - 1)
    }
    const place = (reveal: boolean) => {
      const selected = track.querySelector<HTMLElement>('.gg-segmented__item[data-selected]')
      if (!selected || selected.offsetWidth === 0) {
        pill.toggleAttribute('data-hidden', true)
        fades()
        return
      }
      pill.style.setProperty('--seg-x', `${selected.offsetLeft}px`)
      pill.style.setProperty('--seg-w', `${selected.offsetWidth}px`)
      pill.toggleAttribute('data-hidden', false)
      if (reveal && scroller.scrollWidth > scroller.clientWidth) {
        const edge = 28
        const start = track.offsetLeft + selected.offsetLeft
        const end = start + selected.offsetWidth
        let left = scroller.scrollLeft
        if (start - edge < left) left = Math.max(0, start - edge)
        else if (end + edge > left + scroller.clientWidth) left = end + edge - scroller.clientWidth
        if (left !== scroller.scrollLeft) {
          if (typeof scroller.scrollTo === 'function') scroller.scrollTo({ left, behavior: 'smooth' })
          else scroller.scrollLeft = left
        }
      }
      fades()
    }

    place(true)
    // Animate only after the first placement (no slide-in from x = 0 on mount).
    const frame = requestAnimationFrame(() => pill.toggleAttribute('data-ready', true))

    const observer = typeof ResizeObserver === 'function' ? new ResizeObserver(() => place(false)) : null
    observer?.observe(scroller)
    observer?.observe(track)
    scroller.addEventListener('scroll', fades, { passive: true })
    return () => {
      cancelAnimationFrame(frame)
      observer?.disconnect()
      scroller.removeEventListener('scroll', fades)
    }
  }, [value, layoutKey])

  return { scrollerRef, trackRef, pillRef }
}

/**
 * Shows a picked segment right away even when the owner commits it later (URL search params go through a
 * router transition): the pick is shown while `value` is still the one it replaced, for at most 2 s.
 */
function useOptimisticValue<K extends string>(value: K, onChange: (value: K) => void, enabled = true) {
  const [pending, setPending] = useState<{ from: K; to: K } | null>(null)
  if (pending && pending.from !== value) setPending(null)
  const shown = enabled && pending && pending.from === value ? pending.to : value
  useEffect(() => {
    if (!pending) return
    const timer = window.setTimeout(() => setPending(null), 2000)
    return () => window.clearTimeout(timer)
  }, [pending])
  const select = (next: K) => {
    if (next === shown) return
    if (enabled) setPending(next === value ? null : { from: value, to: next })
    onChange(next)
  }
  return [shown, select] as const
}

function SegmentContent({ item }: { item: SegmentItem }) {
  return (
    <>
      {item.icon ? (
        <span className="gg-segmented__icon" aria-hidden="true">
          {item.icon}
        </span>
      ) : null}
      <span className="gg-segmented__label">{item.label}</span>
      {item.count !== undefined && item.count !== null ? (
        <span className="gg-segmented__count" data-tone={item.countTone ?? 'default'}>
          {item.count}
        </span>
      ) : null}
      {item.adornment}
    </>
  )
}

const layoutKeyOf = (items: readonly SegmentItem[]) =>
  items.map((i) => `${i.id}:${typeof i.label === 'string' ? i.label : ''}:${i.count ?? ''}`).join('|')

/* ------------------------------------------------------------------ SegmentedControl (radiogroup) */

export interface SegmentedControlProps<K extends string> extends Labelling {
  items: readonly SegmentItem<K>[]
  value: K
  onChange: (value: K) => void
  /** Visible label above the control (13 px, like field labels); also names the radiogroup. */
  label?: ReactNode
  size?: Size
  isDisabled?: boolean
  /** Classes for the outer wrapper (scroll container, or the label stack when `label` is set). */
  className?: string
  /**
   * Show a pick at once while the owner commits it later (default). `false` when the owner may refuse or
   * ask first (an unsaved-changes confirm), so a refused pick never lingers (MISTAKES #150).
   */
  optimistic?: boolean
}

export function SegmentedControl<K extends string>({
  items,
  value,
  onChange,
  label,
  size = 'md',
  isDisabled,
  className,
  optimistic = true,
  ...labelling
}: SegmentedControlProps<K>) {
  const labelId = useId()
  const { direction } = useLocale()
  const [selected, select] = useOptimisticValue(value, onChange, optimistic)
  const { scrollerRef, trackRef, pillRef } = useSegmentTrack(selected, layoutKeyOf(items))

  // Radio keyboard model on top of ToggleButtonGroup's toolbar (which only moves focus): runs in the capture
  // phase above the group, so the toolbar never sees the keys we handle.
  const onKeyDownCapture = (e: KeyboardEvent<HTMLDivElement>) => {
    const target = e.target as HTMLElement
    if (!target.classList.contains('gg-segmented__item')) return
    const buttons = Array.from(trackRef.current?.querySelectorAll<HTMLElement>('.gg-segmented__item') ?? [])
    const selectedButton = buttons[items.findIndex((i) => i.id === selected)]
    if (e.key === 'Tab') {
      // One tab stop: leave from the selected segment (the others are out of the tab order).
      e.stopPropagation()
      if (selectedButton && selectedButton !== target) selectedButton.focus()
      return
    }
    const enabled = isDisabled ? [] : items.filter((i) => !i.isDisabled)
    if (enabled.length === 0) return
    const current = Math.max(
      0,
      enabled.findIndex((i) => i.id === selected),
    )
    const next = direction === 'rtl' ? 'ArrowLeft' : 'ArrowRight'
    const prev = direction === 'rtl' ? 'ArrowRight' : 'ArrowLeft'
    let index: number
    if (e.key === next || e.key === 'ArrowDown') index = (current + 1) % enabled.length
    else if (e.key === prev || e.key === 'ArrowUp') index = (current - 1 + enabled.length) % enabled.length
    else if (e.key === 'Home') index = 0
    else if (e.key === 'End') index = enabled.length - 1
    else return
    e.preventDefault()
    e.stopPropagation()
    const item = enabled[index]
    if (!item) return
    select(item.id)
    buttons[items.indexOf(item)]?.focus()
  }

  const control = (
    <div
      ref={scrollerRef}
      className={cx('gg-segmented-scroll', !label && className)}
      onKeyDownCapture={onKeyDownCapture}
    >
      <div ref={trackRef} className="gg-segmented" data-size={size} data-disabled={isDisabled || undefined}>
        <ToggleButtonGroup
          className="gg-segmented__list"
          selectionMode="single"
          disallowEmptySelection
          selectedKeys={[selected]}
          onSelectionChange={(keys) => {
            const next = [...keys][0]
            if (next !== undefined) select(String(next) as K)
          }}
          isDisabled={isDisabled}
          aria-label={labelling['aria-label']}
          aria-labelledby={label ? labelId : labelling['aria-labelledby']}
        >
          {items.map((item) => (
            <ToggleButton
              key={item.id}
              id={item.id}
              className="gg-segmented__item"
              isDisabled={item.isDisabled}
              excludeFromTabOrder={item.id !== selected}
              aria-label={item.textValue}
            >
              <SegmentContent item={item} />
            </ToggleButton>
          ))}
        </ToggleButtonGroup>
        <span ref={pillRef} className="gg-segmented__pill" data-hidden="" aria-hidden="true" />
      </div>
    </div>
  )

  if (!label) return control
  return (
    <div className={cx('flex min-w-0 flex-col gap-1.5', className)}>
      <span id={labelId} className="gg-segmented-label">
        {label}
      </span>
      {control}
    </div>
  )
}

/* ------------------------------------------------------------------ SegmentedTabs (tablist + panels) */

interface SegmentedTabsProps<K extends string> {
  value: K
  onChange: (value: K) => void
  children: ReactNode
  className?: string
  /**
   * Show a pick before the owner commits it (default). Turn off when the owner may refuse or confirm first,
   * or renders only the panel of the committed value.
   */
  optimistic?: boolean
}

/** Content tabs with the segmented look: `List` renders the control, `Panel`s the content. */
export function SegmentedTabs<K extends string>({
  value,
  onChange,
  children,
  className,
  optimistic = true,
}: SegmentedTabsProps<K>) {
  const [selected, select] = useOptimisticValue(value, onChange, optimistic)
  return (
    <Tabs selectedKey={selected} onSelectionChange={(key) => select(String(key) as K)} className={className}>
      {children}
    </Tabs>
  )
}

interface SegmentedTabListProps<K extends string> extends Labelling {
  items: readonly SegmentItem<K>[]
  size?: Size
  className?: string
}

function SegmentedTabList<K extends string>({
  items,
  size = 'md',
  className,
  ...labelling
}: SegmentedTabListProps<K>) {
  const state = useContext(TabListStateContext)
  const selected = state?.selectedKey == null ? null : String(state.selectedKey)
  const { scrollerRef, trackRef, pillRef } = useSegmentTrack(selected, layoutKeyOf(items))
  return (
    <div ref={scrollerRef} className={cx('gg-segmented-scroll', className)}>
      <div ref={trackRef} className="gg-segmented" data-size={size}>
        <TabList className="gg-segmented__list" {...labelling}>
          {items.map((item) => (
            <Tab
              key={item.id}
              id={item.id}
              className="gg-segmented__item"
              isDisabled={item.isDisabled}
              aria-label={item.textValue}
            >
              <SegmentContent item={item} />
            </Tab>
          ))}
        </TabList>
        <span ref={pillRef} className="gg-segmented__pill" data-hidden="" aria-hidden="true" />
      </div>
    </div>
  )
}

function SegmentedTabPanel({ className, ...props }: TabPanelProps & { className?: string }) {
  return <TabPanel {...props} className={cx('gg-tab-panel', className)} />
}

SegmentedTabs.List = SegmentedTabList
SegmentedTabs.Panel = SegmentedTabPanel
