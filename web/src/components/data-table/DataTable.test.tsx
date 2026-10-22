import { render, screen } from '@testing-library/react'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import { loadNamespaces } from '@/i18n'
import { DataTable } from './DataTable'
import { createDataTableColumns } from './features'
import { pageWindow, parseSort, toSortParam } from './sort'

interface Row {
  id: number
  email: string
  balance: number
}
const col = createDataTableColumns<Row>()
const columns = col.columns([
  col.accessor('email', { header: 'Email', meta: { isPrimary: true } }),
  col.accessor('balance', { header: 'Balance', meta: { align: 'end' } }),
])
const rows: Row[] = [
  { id: 1, email: 'a@example.com', balance: 100 },
  { id: 2, email: 'b@example.com', balance: 200 },
]

function setDesktop(matches: boolean) {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches,
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }))
}

describe('DataTable', () => {
  beforeAll(async () => {
    await loadNamespaces(['common', 'core'], 'zh-CN')
  })

  it('renders a HeroUI table with rows and a pagination summary on desktop', async () => {
    setDesktop(true)
    render(
      <DataTable
        aria-label="Users"
        columns={columns}
        data={rows}
        total={42}
        page={2}
        pageSize={20}
        getRowId={(r) => String(r.id)}
        onPageChange={() => undefined}
      />,
    )
    expect(await screen.findByRole('grid', { name: 'Users' })).toBeInTheDocument()
    expect(screen.getByText('a@example.com')).toBeInTheDocument()
    expect(screen.getByText('第 21–40 条，共 42 条')).toBeInTheDocument()
  })

  it('renders cards on mobile and an empty state without rows', async () => {
    setDesktop(false)
    const { rerender } = render(<DataTable aria-label="Users" columns={columns} data={rows} />)
    expect(await screen.findByRole('list', { name: 'Users' })).toBeInTheDocument()
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    rerender(<DataTable aria-label="Users" columns={columns} data={[]} />)
    expect(screen.getByText('暂无数据')).toBeInTheDocument()
  })
})

describe('sort + pagination helpers', () => {
  it('round-trips the API sort param', () => {
    expect(parseSort('-created_at')).toEqual([{ id: 'created_at', desc: true }])
    expect(parseSort('email')).toEqual([{ id: 'email', desc: false }])
    expect(toSortParam([{ id: 'created_at', desc: true }])).toBe('-created_at')
    expect(toSortParam([])).toBeUndefined()
  })

  it('windows page numbers with ellipses', () => {
    expect(pageWindow(1, 3)).toEqual([1, 2, 3])
    expect(pageWindow(10, 20)).toEqual([1, 'ellipsis', 9, 10, 11, 'ellipsis', 20])
    expect(pageWindow(2, 20)).toEqual([1, 2, 3, 'ellipsis', 20])
  })
})
