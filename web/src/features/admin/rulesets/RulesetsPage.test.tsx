import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { loadNamespaces } from '@/i18n'
import { useUiStore } from '@/stores/ui'
import { RulesetsPage } from './RulesetsPage'

const builtin = {
  meta: { generated_at: '2026-10-01T00:00:00Z', version: 1 },
  sources: {
    acme: { name: 'acme/rules', order: 10, urls: { clash: 'https://example.com/{name}.yaml' } },
  },
  categories: [],
  templates: [],
  mirrors: [],
}

const json = (data: unknown) =>
  new Response(JSON.stringify({ data }), { status: 200, headers: { 'Content-Type': 'application/json' } })

describe('RulesetsPage', () => {
  beforeAll(async () => {
    useUiStore.setState({ locale: 'en' })
    await loadNamespaces(['admin-settings', 'common', 'core'], 'en')
  })
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof fetch>(() => Promise.resolve(json({ builtin, effective: builtin, custom: null }))),
    )
  })
  afterEach(() => vi.unstubAllGlobals())

  it('shows facts on the first level and keeps the explanations behind ⓘ buttons', async () => {
    const user = userEvent.setup()
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={qc}>
        <RulesetsPage tab="custom" onTabChange={() => undefined} />
      </QueryClientProvider>,
    )
    // The subtitle is the catalog's generation date; what the page is for opens from the title's ⓘ.
    expect(
      await screen.findByText('Built-in catalog generated on 2026-10-01 · schema v1'),
    ).toBeInTheDocument()
    expect(screen.queryByText(/effective catalog/)).toBeNull()
    await user.click(screen.getByRole('button', { name: 'About “Rule sets”' }))
    expect(await screen.findByRole('dialog')).toHaveTextContent('effective catalog')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

    // How overrides merge: behind the editor title's ⓘ.
    expect(screen.queryByText(/merge by key/)).toBeNull()
    await user.click(screen.getByRole('button', { name: 'About “Custom overrides”' }))
    expect(await screen.findByRole('dialog')).toHaveTextContent('merge by key')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

    // The per-source rules keep a clean group name; how to fill them is in the ⓘ beside it.
    await user.click(screen.getByRole('button', { name: 'Add a custom category' }))
    const refs = await screen.findByRole('group', { name: 'Rules per source' })
    expect(within(refs).getByRole('textbox', { name: 'File name (name)' })).toBeInTheDocument()
    expect(screen.queryByText(/empty sources are skipped/)).toBeNull()
    await user.click(within(refs).getByRole('button', { name: 'About “Rules per source”' }))
    expect(await screen.findByRole('dialog')).toHaveTextContent('empty sources are skipped')
    await user.keyboard('{Escape}')
  })
})
