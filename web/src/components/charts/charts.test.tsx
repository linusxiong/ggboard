import { fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { BarChart } from './BarChart'
import { LineChart } from './LineChart'
import { byteUnit, columnPath, niceTicks, niceTicksIn } from './scale'
import { Sparkline } from './Sparkline'

const labels = ['2026-10-20', '2026-10-21', '2026-10-22']
const money = (v: number) => `¥${(v / 100).toFixed(2)}`

describe('chart scales', () => {
  it('computes round ticks', () => {
    expect(niceTicks(0, 87)).toEqual([0, 25, 50, 75, 100])
    expect(niceTicks(0, 180)).toEqual([0, 50, 100, 150, 200])
    expect(niceTicks(0, 0)).toEqual([0, 0.25, 0.5, 0.75, 1])
    expect(niceTicks(-5, 5)).toEqual([-5, -2.5, 0, 2.5, 5])
    const gib = 1024 ** 3
    expect(byteUnit(186 * gib)).toBe(gib)
    expect(byteUnit(512)).toBe(1)
    expect(niceTicksIn(0, 186 * gib, gib)).toEqual([0, 50, 100, 150, 200].map((v) => v * gib))
    // whole raw units: an empty series must not produce duplicate labels (0.25 B → "0 B", "1 B")
    expect(niceTicksIn(0, 0)).toEqual([0, 1])
    expect(niceTicksIn(0, 3)).toEqual([0, 1, 2, 3])
    expect(niceTicksIn(0, 512 * 1024 ** 2, 1024 ** 2)).toEqual([0, 200, 400, 600].map((v) => v * 1024 ** 2))
  })

  it('draws columns with a rounded data end and square base', () => {
    expect(columnPath(0, 10, 20, 30)).toBe('M0 40 L0 14 Q0 10 4 10 L16 10 Q20 10 20 14 L20 40 Z')
    expect(columnPath(0, 10, 20, 0)).toBe('')
  })
})

describe('chart accessibility fallback', () => {
  it('LineChart: named image + hidden data table + keyboard readout', () => {
    render(
      <LineChart
        aria-label="收入（近 3 天）"
        labels={labels}
        series={[{ id: 'income', label: '收入', values: [1000, 2550, 0] }]}
        formatValue={money}
        labelHeader="日期"
      />,
    )
    const img = screen.getByRole('img', { name: '收入（近 3 天）' })
    const table = screen.getByRole('table', { name: '收入（近 3 天）' })
    expect(table).toHaveClass('sr-only')
    expect(within(table).getAllByRole('row')).toHaveLength(4)
    expect(within(table).getByRole('columnheader', { name: '日期' })).toBeInTheDocument()
    expect(within(table).getByRole('rowheader', { name: '2026-10-21' })).toBeInTheDocument()
    expect(within(table).getByText('¥25.50')).toBeInTheDocument()

    expect(screen.queryByTestId('chart-tooltip')).toBeNull()
    fireEvent.focus(img)
    expect(screen.getByTestId('chart-tooltip')).toHaveTextContent('¥0.00')
    fireEvent.keyDown(img, { key: 'ArrowLeft' })
    expect(screen.getByTestId('chart-tooltip')).toHaveTextContent('2026-10-21')
    fireEvent.keyDown(img, { key: 'Home' })
    expect(screen.getByTestId('chart-tooltip')).toHaveTextContent('¥10.00')
    fireEvent.keyDown(img, { key: 'Escape' })
    expect(screen.queryByTestId('chart-tooltip')).toBeNull()
  })

  it('BarChart: stacked series get a legend and a column per series in the table', () => {
    render(
      <BarChart
        aria-label="节点流量"
        labels={labels}
        stacked
        series={[
          { id: 'u', label: '上行', values: [1, 2, 3] },
          { id: 'd', label: '下行', values: [4, 5, 6] },
        ]}
      />,
    )
    expect(screen.getByRole('img', { name: '节点流量' })).toBeInTheDocument()
    const table = screen.getByRole('table', { name: '节点流量' })
    expect(
      within(table)
        .getAllByRole('columnheader')
        .map((th) => th.textContent),
    ).toEqual(['—', '上行', '下行'])
    expect(screen.getByRole('list')).toHaveTextContent('上行下行') // legend for ≥ 2 series
  })

  it('renders an empty state without data', () => {
    render(
      <LineChart
        aria-label="空"
        labels={[]}
        series={[{ id: 'a', label: 'A', values: [] }]}
        emptyLabel="暂无数据"
      />,
    )
    expect(screen.getByText('暂无数据')).toBeInTheDocument()
    expect(screen.queryByRole('img')).toBeNull()
  })

  it('Sparkline is a labelled image', () => {
    render(<Sparkline values={[1, 3, 2]} aria-label="近 7 天：从 1 到 2" />)
    expect(screen.getByRole('img', { name: '近 7 天：从 1 到 2' })).toBeInTheDocument()
  })
})
