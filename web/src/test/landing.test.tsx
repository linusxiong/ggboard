import { screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { makePlan } from '@/features/shop/testing'
import { GUEST_CONFIG, mockApi, renderAt, resetStores } from './fe1-helpers'

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
