import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { loadNamespaces } from '@/i18n'
import type { AdminSettingsResponse, SettingGroupMeta } from '@/lib/api/types'
import { useUiStore } from '@/stores/ui'
import { SettingsPage } from './SettingsPage'
import { buildGroups, dirtyPatch, flattenValues, randomToken, sameValue } from './settingsMeta'

const groups: SettingGroupMeta[] = [
  {
    name: 'site',
    keys: [
      { key: 'app_name', type: 'string', default: 'GG Bond' },
      { key: 'default_locale', type: 'string', default: 'zh-CN', options: ['zh-CN', 'en'] },
      { key: 'show_info_to_user', type: 'bool', default: true },
      { key: 'subscribe_update_interval', type: 'int', default: 3, min: 1, max: 100 },
      { key: 'captcha_secret', type: 'string', default: '', secret: true },
      { key: 'email_whitelist_suffix', type: 'strings', default: [] },
      { key: 'custom_json', type: 'json', default: {} },
      { key: 'big_counter', type: 'int64', default: 0 },
      { key: 'rulesets_custom', type: 'json', default: {}, hidden: true },
    ],
  },
  {
    name: 'email',
    keys: [{ key: 'email_driver', type: 'string', default: 'log', options: ['log', 'smtp'] }],
  },
]

const response: AdminSettingsResponse = {
  site: {
    app_name: 'GG Bond',
    default_locale: 'zh-CN',
    show_info_to_user: true,
    subscribe_update_interval: 3,
    captcha_secret: 's3cret',
    email_whitelist_suffix: ['example.com'],
    custom_json: { a: 1 },
    big_counter: 7,
  },
  email: { email_driver: 'log' },
  groups,
}

function json(data: unknown, status = 200) {
  return new Response(JSON.stringify(status < 300 ? { data } : data), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

const urlOf = (input: RequestInfo | URL) => (input instanceof Request ? input.url : input.toString())
const bodyOf = (init?: RequestInit) => (typeof init?.body === 'string' ? init.body : '')

describe('settings metadata helpers', () => {
  it('normalises groups (hidden keys dropped, int64 → int) and flattens values', () => {
    const built = buildGroups(response)
    expect(built.map((g) => g.name)).toEqual(['site', 'email'])
    const keys = built[0]!.keys.map((k) => k.key)
    expect(keys).not.toContain('rulesets_custom')
    expect(built[0]!.keys.find((k) => k.key === 'big_counter')?.type).toBe('int')
    const flat = flattenValues(response, built)
    expect(flat.app_name).toBe('GG Bond')
    expect(flat.email_driver).toBe('log')
  })

  it('infers metadata when the backend sends no `groups`', () => {
    const built = buildGroups({
      email: { email_password: 'x', email_port: 465 },
      site: { show_info_to_user: true },
    })
    expect(built.map((g) => g.name)).toEqual(['site', 'email'])
    expect(built[1]!.keys).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ key: 'email_password', type: 'string', secret: true }),
        expect.objectContaining({ key: 'email_port', type: 'int' }),
      ]),
    )
  })

  it('builds a patch with only changed keys and validates typed values', () => {
    const built = buildGroups(response)
    const initial = flattenValues(response, built)
    const msg = {
      json: 'bad json',
      number: 'nan',
      min: (n: number) => `min ${n}`,
      max: (n: number) => `max ${n}`,
    }
    const { patch, errors } = dirtyPatch(
      built[0]!,
      initial,
      {
        app_name: 'GG Bond',
        subscribe_update_interval: 0,
        custom_json: '{"a":1}',
        email_whitelist_suffix: ['a.com'],
      },
      msg,
    )
    expect(patch).toEqual({ email_whitelist_suffix: ['a.com'] })
    expect(errors).toEqual({ subscribe_update_interval: 'min 1' })
    expect(dirtyPatch(built[0]!, initial, { custom_json: '{' }, msg).errors).toEqual({
      custom_json: 'bad json',
    })
    expect(sameValue({ a: [1, 2] }, { a: [1, 2] })).toBe(true)
  })

  it('generates 32-character alphanumeric tokens', () => {
    const token = randomToken()
    expect(token).toMatch(/^[A-Za-z0-9]{32}$/)
    expect(randomToken()).not.toBe(token)
  })
})

describe('SettingsPage', () => {
  const fetchMock = vi.fn<typeof fetch>()
  let putBodies: unknown[] = []

  beforeAll(async () => {
    useUiStore.setState({ locale: 'en' })
    await loadNamespaces(['admin-settings', 'common', 'core'], 'en')
  })

  beforeEach(() => {
    putBodies = []
    fetchMock.mockImplementation((input, init) => {
      const url = urlOf(input)
      const method = init?.method ?? 'GET'
      if (url.includes('/admin/settings') && method === 'GET') return Promise.resolve(json(response))
      if (url.includes('/admin/settings') && method === 'PUT') {
        const body = JSON.parse(bodyOf(init)) as Record<string, unknown>
        putBodies.push(body)
        if ('subscribe_update_interval' in body && body.subscribe_update_interval === 99)
          return Promise.resolve(
            json(
              {
                code: 'validation_failed',
                message: 'invalid',
                errors: { subscribe_update_interval: ['too many'] },
              },
              400,
            ),
          )
        return Promise.resolve(json(true))
      }
      if (url.includes('/admin/rulesets'))
        return Promise.resolve(json({ builtin: { sources: {}, categories: [], templates: [] }, custom: {} }))
      if (url.includes('/admin/plans')) return Promise.resolve(json([]))
      return Promise.resolve(json({ code: 'not_found', message: 'nope' }, 404))
    })
    vi.stubGlobal('fetch', fetchMock)
  })
  afterEach(() => vi.unstubAllGlobals())

  function renderPage() {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    return render(
      <QueryClientProvider client={qc}>
        <SettingsPage group="site" onGroupChange={() => undefined} />
      </QueryClientProvider>,
    )
  }

  it('renders every metadata type and sends only the dirty keys', async () => {
    const user = userEvent.setup()
    renderPage()
    const panel = await screen.findByRole('form', { name: 'Site' })

    // string → text field, enum → select, bool → switch, int → number field, secret → password,
    // strings → tag group, json → textarea.
    expect(within(panel).getByRole('textbox', { name: 'Site name' })).toHaveValue('GG Bond')
    expect(within(panel).getByText('Default language', { selector: 'label, span' })).toBeInTheDocument()
    expect(within(panel).getByRole('switch', { name: 'Show node addresses to users' })).toBeChecked()
    expect(within(panel).getByRole('textbox', { name: 'Update interval (hours)' })).toHaveValue('3')
    expect(within(panel).getByLabelText('Secret key')).toHaveAttribute('type', 'password')
    expect(within(panel).getByRole('grid', { name: 'Allowed email domains' })).toBeInTheDocument()
    expect(within(panel).getByText('example.com')).toBeInTheDocument()
    expect(within(panel).getByRole('textbox', { name: 'custom_json' })).toHaveValue('{\n  "a": 1\n}')
    expect(within(panel).getByRole('textbox', { name: 'big_counter' })).toHaveValue('7')

    const save = within(panel).getByRole('button', { name: 'Save' })
    expect(save).toBeDisabled()

    await user.click(within(panel).getByRole('switch', { name: 'Show node addresses to users' }))
    const name = within(panel).getByRole('textbox', { name: 'Site name' })
    await user.clear(name)
    await user.type(name, 'My Panel')
    expect(save).toBeEnabled()
    await user.click(save)

    await waitFor(() => expect(putBodies).toHaveLength(1))
    expect(putBodies[0]).toEqual({ app_name: 'My Panel', show_info_to_user: false })
  })

  it('shows server field errors inline (400 errors keyed by setting key)', async () => {
    const user = userEvent.setup()
    renderPage()
    const panel = await screen.findByRole('form', { name: 'Site' })
    const count = within(panel).getByRole('textbox', { name: 'Update interval (hours)' })
    await user.clear(count)
    await user.type(count, '99')
    await user.tab()
    await user.click(within(panel).getByRole('button', { name: 'Save' }))
    expect(await within(panel).findByText('too many')).toBeInTheDocument()
    expect(putBodies[0]).toEqual({ subscribe_update_interval: 99 })
  })
})
