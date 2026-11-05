import { screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { makePlan } from '@/features/shop/testing'
import { GUEST_CONFIG, mockApi, renderAt, resetStores, signIn } from './fe1-helpers'

describe('landing plans', () => {
  beforeEach(() => {
    resetStores()
    mockApi({
      'GET /guest/config': GUEST_CONFIG,
      'GET /guest/plans': [
        makePlan({ id: 1, name: 'Lite', device_limit: 3, conn_limit: null }),
        makePlan({ id: 2, name: 'Pro', device_limit: 5, conn_limit: 64 }),
        makePlan({ id: 3, name: 'Ultra', device_limit: null, conn_limit: 0 }),
      ],
    })
  })
  afterEach(() => vi.unstubAllGlobals())

  it('lists the connection limit only on plans that set one', async () => {
    renderAt('/')
    const card = async (name: string) =>
      within((await screen.findByRole('heading', { level: 3, name }, { timeout: 8000 })).closest('li')!)

    expect((await card('Pro')).getByText('单节点 64 个连接')).toBeInTheDocument()
    expect((await card('Pro')).getByText('5 台设备')).toBeInTheDocument()
    // null and 0 both mean unlimited: no connection line.
    expect((await card('Lite')).queryByText(/个连接/)).not.toBeInTheDocument()
    expect((await card('Ultra')).queryByText(/个连接/)).not.toBeInTheDocument()
    expect((await card('Ultra')).getByText('不限设备')).toBeInTheDocument()
  })
})

describe('landing session', () => {
  beforeEach(() => {
    resetStores()
    mockApi({ 'GET /guest/config': GUEST_CONFIG, 'GET /guest/plans': [] })
  })
  afterEach(() => vi.unstubAllGlobals())

  const link = (name: string) => screen.findByRole('link', { name }, { timeout: 8000 })

  it('offers registration and sign-in to a guest', async () => {
    const { container } = renderAt('/')
    expect(await link('登录')).toHaveAttribute('href', '/login')
    expect(await link('立即开始')).toHaveAttribute('href', '/register')
    expect(await link('已有账户登录')).toHaveAttribute('href', '/login')
    expect(screen.queryByRole('link', { name: '进入控制台' })).not.toBeInTheDocument()
    expect(container.querySelector('[data-session-only]')).toBeNull()
  })

  it('leads a signed-in visitor to the console', async () => {
    signIn()
    const { container } = renderAt('/')
    expect(await link('进入控制台')).toHaveAttribute('href', '/dashboard')
    expect(await link('立即开始')).toHaveAttribute('href', '/dashboard')
    expect(screen.queryByRole('link', { name: '登录' })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: '已有账户登录' })).not.toBeInTheDocument()
    expect(container.querySelector('[data-session-only]')).toBeNull()
  })

  it('prerenders both variants for the boot script to choose from', async () => {
    vi.stubGlobal('__GG_PRERENDER__', true)
    const { container } = renderAt('/')
    await link('进入控制台')
    const links = (only: string) =>
      [...container.querySelectorAll(`[data-session-only="${only}"] a`)].map((a) => [
        a.textContent,
        a.getAttribute('href'),
      ])

    expect(links('guest')).toEqual([
      ['登录', '/login'],
      ['立即开始', '/register'],
      ['已有账户登录', '/login'],
    ])
    expect(links('member')).toEqual([
      ['进入控制台', '/dashboard'],
      ['立即开始', '/dashboard'],
    ])
  })
})
