import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it } from 'vitest'
import { callsTo, mockApi, renderApp, resetApp, urlOfCall } from '@/features/shop/testing'

const ARTICLES = [
  {
    id: 1,
    category: '入门',
    title: '快速开始',
    body: '[一键导入](clash://install-config?url=x) [坏链接](javascript:alert(1))',
  },
  { id: 2, category: '客户端', title: 'Clash Verge', body: 'Clash 教程' },
  {
    id: 3,
    category: '客户端',
    title: 'Shadowrocket',
    body: '前言\n\n<div class="knowledge-no-access">你必须拥有有效的订阅才可以查看该区域的内容</div>',
  },
]

describe('knowledge', () => {
  afterEach(resetApp)

  it('searches (debounced) and filters the index by keyword', async () => {
    const fetchMock = mockApi({
      'GET /user/knowledge': ({ url }) => {
        const keyword = (url.searchParams.get('keyword') ?? '').toLowerCase()
        const index: Record<string, { id: number; title: string; updated_at: string }[]> = {}
        for (const a of ARTICLES) {
          if (keyword && !`${a.title} ${a.body}`.toLowerCase().includes(keyword)) continue
          ;(index[a.category] ??= []).push({ id: a.id, title: a.title, updated_at: '2026-10-01T00:00:00Z' })
        }
        return index
      },
    })
    const { router } = renderApp('/knowledge')
    const u = userEvent.setup()

    expect(await screen.findByRole('link', { name: '快速开始' }, { timeout: 4000 })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Shadowrocket' })).toBeInTheDocument()
    // Language defaults to the UI locale.
    expect(urlOfCall(callsTo(fetchMock, 'GET', '/user/knowledge')[0])?.searchParams.get('language')).toBe(
      'zh-CN',
    )

    await u.type(screen.getByRole('searchbox', { name: '搜索文档' }), 'clash')
    await waitFor(() => expect(router.state.location.search).toMatchObject({ keyword: 'clash' }), {
      timeout: 3000,
    })
    await waitFor(() => expect(screen.queryByRole('link', { name: 'Shadowrocket' })).not.toBeInTheDocument())
    expect(screen.getByRole('link', { name: 'Clash Verge' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '快速开始' })).toBeInTheDocument()
    // One request per settled keyword, not per keystroke.
    const keywords = callsTo(fetchMock, 'GET', '/user/knowledge').map((call) =>
      urlOfCall(call)?.searchParams.get('keyword'),
    )
    expect(keywords.filter((k) => k !== null && k !== undefined)).toEqual(['clash'])

    await u.clear(screen.getByRole('searchbox', { name: '搜索文档' }))
    await u.type(screen.getByRole('searchbox', { name: '搜索文档' }), 'zzz')
    expect(await screen.findByText('没有找到与「zzz」相关的文档', {}, { timeout: 3000 })).toBeInTheDocument()
  })

  it('renders an article: keeps client deep links, strips javascript:, styles gated blocks', async () => {
    mockApi({
      'GET /user/knowledge': () => ({
        入门: [{ id: 1, title: '快速开始', updated_at: '2026-10-01T00:00:00Z' }],
      }),
      'GET /user/knowledge/1': () => ({
        ...ARTICLES[0],
        language: 'zh-CN',
        updated_at: '2026-10-01T00:00:00Z',
      }),
      'GET /user/knowledge/3': () => ({
        ...ARTICLES[2],
        language: 'zh-CN',
        updated_at: '2026-10-01T00:00:00Z',
      }),
    })
    const { router } = renderApp('/knowledge?id=1')

    const deepLink = await screen.findByText('一键导入', {}, { timeout: 4000 })
    expect(deepLink.closest('a')).toHaveAttribute('href', 'clash://install-config?url=x')
    expect(screen.getByText('坏链接').closest('a')).not.toHaveAttribute('href')

    await router.navigate({ to: '/knowledge', search: { id: 3 } })
    expect(
      await screen.findByText('部分内容需要有效的套餐才能查看。', {}, { timeout: 4000 }),
    ).toBeInTheDocument()
    expect(within(screen.getByRole('article')).getByRole('link', { name: '购买套餐' })).toHaveAttribute(
      'href',
      '/shop',
    )
    const gated = await screen.findByText('你必须拥有有效的订阅才可以查看该区域的内容')
    expect(gated).toHaveClass('knowledge-no-access')
  })
})
