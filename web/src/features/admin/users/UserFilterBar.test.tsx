/** Staff never see UUIDs or subscription tokens, so the advanced filters do not offer them either. */
import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { loadNamespaces } from '@/i18n'
import type { User } from '@/lib/api/types'
import { createQueryClient } from '@/lib/queryClient'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import fixtureUser from '../../../../scripts/fixtures/user.json'
import { UserFilterBar } from './UserFilterBar'

function renderBar() {
  return render(
    <QueryClientProvider client={createQueryClient()}>
      <UserFilterBar
        search={{ page: 1, page_size: 20, remarks: 'vip' }}
        onChange={() => {}}
        onReset={() => {}}
      />
    </QueryClientProvider>,
  )
}

describe('user filter bar', () => {
  beforeAll(async () => {
    useUiStore.setState({ locale: 'zh-CN' })
    await loadNamespaces(['admin-users', 'common'])
  })
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(JSON.stringify({ data: [] }), { headers: { 'Content-Type': 'application/json' } }),
        ),
      ),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    useAuthStore.setState({ token: null, user: null })
  })

  it('offers the UUID and token filters to administrators only', async () => {
    useAuthStore.setState({ token: 't', user: { ...(fixtureUser as User), is_admin: true, is_staff: false } })
    const admin = renderBar()
    expect(await screen.findByLabelText('UUID')).toBeInTheDocument()
    expect(screen.getByLabelText('订阅 Token')).toBeInTheDocument()
    admin.unmount()

    useAuthStore.setState({ token: 't', user: { ...(fixtureUser as User), is_admin: false, is_staff: true } })
    renderBar()
    expect(await screen.findByDisplayValue('vip')).toBeInTheDocument()
    expect(screen.queryByLabelText('UUID')).toBeNull()
    expect(screen.queryByLabelText('订阅 Token')).toBeNull()
  })
})
