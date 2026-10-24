import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { ConfirmHost } from '@/components/feedback/ConfirmHost'
import { loadNamespaces, t } from '@/i18n'
import type { AdminMailTemplate, AdminMailTemplateSummary, MailTemplatePreview, User } from '@/lib/api/types'
import { createQueryClient } from '@/lib/queryClient'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import fixtureUser from '../../../../scripts/fixtures/user.json'
import { type MailTemplateSelection, MailTemplatesPage } from './MailTemplatesPage'
import {
  editorVariables,
  LABELLED_TEMPLATES,
  LABELLED_VARIABLES,
  TEMPLATE_URL_LABELS,
  asPlaceholder,
} from './variables'

const TOKEN = 'header.eyJleHAiOjQxMDI0NDQ4MDB9.sig'

const SUMMARIES: AdminMailTemplateSummary[] = [
  {
    name: 'verify_code',
    variables: [
      { name: 'code', required: true },
      { name: 'minutes', required: false },
    ],
    locales: {
      'zh-CN': { custom: true, updated_at: '2026-10-20T08:00:00Z' },
      en: { custom: false, updated_at: null },
    },
  },
  {
    name: 'notify',
    variables: [
      { name: 'content', required: true },
      { name: 'title', required: false },
    ],
    locales: { 'zh-CN': { custom: false, updated_at: null }, en: { custom: false, updated_at: null } },
  },
]
const DEFAULT = { subject: '【{{siteName}}】邮箱验证码', body: '## 邮箱验证码\n\n{{code}}' }
const ZH: AdminMailTemplate = {
  name: 'verify_code',
  locale: 'zh-CN',
  custom: true,
  subject: '【{{siteName}}】验证码',
  body: '验证码：{{code}}',
  updated_at: '2026-10-20T08:00:00Z',
  default: DEFAULT,
}
const EN: AdminMailTemplate = {
  name: 'verify_code',
  locale: 'en',
  custom: false,
  subject: '{{siteName}} verification code',
  body: 'Your code: {{code}}',
  updated_at: null,
  default: { subject: '{{siteName}} verification code', body: 'Your code: {{code}}' },
}
const OK_PREVIEW: MailTemplatePreview = {
  subject: '【GG Bond】验证码',
  html: '<!doctype html><p>验证码：428913</p>',
  text: '验证码：428913',
  unknown: [],
  missing: [],
}

interface Call {
  method: string
  path: string
  query: URLSearchParams
  body: unknown
}
type Handler = (call: Call) => Response | undefined

const json = (data: unknown, status = 200) =>
  new Response(JSON.stringify(status < 400 ? { data } : data), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })

/** fetch mock: `handler` first, then the default template endpoints; every request is recorded. */
function mockApi(handler: Handler = () => undefined) {
  const calls: Call[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(input instanceof Request ? input.url : input.toString(), 'http://localhost')
      const call: Call = {
        method: (init?.method ?? 'GET').toUpperCase(),
        path: url.pathname.replace(/^\/api\/v1/, ''),
        query: url.searchParams,
        body: typeof init?.body === 'string' ? JSON.parse(init.body) : undefined,
      }
      calls.push(call)
      const custom = handler(call)
      if (custom) return Promise.resolve(custom)
      const key = `${call.method} ${call.path}`
      if (key === 'GET /admin/mail-templates') return Promise.resolve(json(SUMMARIES))
      if (key === 'GET /admin/mail-templates/verify_code')
        return Promise.resolve(json(call.query.get('locale') === 'en' ? EN : ZH))
      if (key === 'POST /admin/mail-templates/verify_code/preview') return Promise.resolve(json(OK_PREVIEW))
      return Promise.resolve(json({ code: 'not_found', message: key }, 404))
    }),
  )
  return calls
}

/** Plays the route: the selection lives in state like it lives in the URL. */
function Harness() {
  const [selection, setSelection] = useState<Partial<MailTemplateSelection>>({})
  return <MailTemplatesPage name={selection.name} locale={selection.locale} onSelect={setSelection} />
}

function renderPage() {
  const client = createQueryClient()
  const view = render(
    <QueryClientProvider client={client}>
      <Harness />
      <ConfirmHost />
    </QueryClientProvider>,
  )
  return { client, view }
}

const subjectBox = () => screen.findByRole('textbox', { name: /^邮件标题/ }, { timeout: 4000 })
const bodyBox = () => screen.getByRole('textbox', { name: /^邮件正文/ })

describe('mail template helpers', () => {
  it('adds the common variables and puts required ones first', () => {
    expect(
      editorVariables([
        { name: 'siteName', required: false },
        { name: 'url', required: false },
        { name: 'code', required: true },
      ]).map((v) => v.name),
    ).toEqual(['code', 'url', 'siteName', 'siteUrl', 'year'])
    expect(asPlaceholder('{{ code }}')).toBe('{{ code }}')
    expect(asPlaceholder('bar')).toBe('{{bar}}')
  })

  it('has a label for every labelled template and variable in both locales', async () => {
    const keys = [
      ...[...LABELLED_VARIABLES].map((n) => `mailTemplates.vars.${n}`),
      ...[...LABELLED_TEMPLATES].flatMap((n) => [
        `mailTemplates.templates.${n}.title`,
        `mailTemplates.templates.${n}.description`,
      ]),
      ...[...TEMPLATE_URL_LABELS].map((n) => `mailTemplates.templateVars.${n}.url`),
    ]
    for (const locale of ['zh-CN', 'en'] as const) {
      await loadNamespaces(['admin-settings'], locale)
      for (const key of keys)
        expect(t('admin-settings', key, undefined, locale), `${locale} ${key}`).not.toBe(key)
    }
  })
})

describe('MailTemplatesPage', () => {
  beforeAll(async () => {
    useUiStore.setState({ locale: 'zh-CN' })
    await loadNamespaces(['admin-settings', 'admin-support', 'common'])
  })
  beforeEach(() => {
    useAuthStore.setState({ token: TOKEN, user: { ...(fixtureUser as User), is_admin: true } })
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    useAuthStore.setState({ token: null, user: null })
  })

  it('lists the templates with their per-locale state', async () => {
    mockApi()
    const { client } = renderPage()
    const nav = await screen.findByRole('navigation', { name: '邮件模板' }, { timeout: 4000 })
    const current = within(nav).getByRole('button', { current: 'page' })
    expect(current).toHaveTextContent('邮箱验证码')
    expect(current).toHaveTextContent('中文 已自定义')
    expect(current).toHaveTextContent('英文 默认')
    expect(within(nav).getByRole('button', { name: /通用通知/ })).toBeInTheDocument()
    expect(await subjectBox()).toHaveValue(ZH.subject)
    client.clear()
  })

  it('keeps explanations behind ⓘ buttons (page, template, variables, preview)', async () => {
    mockApi()
    const { client } = renderPage()
    await subjectBox()
    const user = userEvent.setup()
    const opens = async (name: string, text: string) => {
      await user.click(screen.getByRole('button', { name }))
      expect(await screen.findByRole('dialog')).toHaveTextContent(text)
      await user.keyboard('{Escape}')
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    }
    // What the template is for: not in the list, behind the title's ⓘ.
    const purpose = '注册、找回密码时发送的验证码。'
    expect(screen.queryByText(purpose)).toBeNull()
    await opens('关于「邮件模板」的说明', '未自定义时使用内置模板')
    await opens('关于「邮箱验证码」的说明', purpose)
    // The placeholder reference is opened from the end of the toolbar.
    expect(screen.queryByText(/点击变量按钮在光标处插入/)).toBeNull()
    await user.click(
      within(screen.getByRole('toolbar', { name: '格式工具' })).getByRole('button', {
        name: '关于「邮件正文」的说明',
      }),
    )
    const vars = within(await screen.findByRole('dialog')).getByRole('list', { name: '可用变量' })
    expect(vars).toHaveTextContent('{{code}}验证码')
    expect(vars).toHaveTextContent('{{minutes}}有效期（分钟）')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    await opens('关于「实时预览」的说明', '使用示例数据渲染')
    client.clear()
  })

  it('inserts {{variable}} at the cursor from the toolbar buttons', async () => {
    mockApi()
    const { client } = renderPage()
    await subjectBox()
    const body = bodyBox() as HTMLTextAreaElement
    const user = userEvent.setup()
    await user.click(body)
    const at = '验证码：'.length
    body.setSelectionRange(at, at)
    const toolbar = within(screen.getByRole('toolbar', { name: '格式工具' }))
    // Required variables are marked (accessible name says so; the `*` is decorative).
    await user.click(toolbar.getByRole('button', { name: '插入 {{minutes}}：有效期（分钟）' }))
    expect(body).toHaveValue('验证码：{{minutes}}{{code}}')
    expect(toolbar.getByRole('button', { name: '插入 {{code}}：验证码（必填）' })).toHaveTextContent(
      '{{code}}*',
    )
    expect(toolbar.getByRole('button', { name: '插入 {{siteName}}：站点名称' })).toBeInTheDocument()
    client.clear()
  })

  it('shows missing and unknown placeholders from the preview inline', async () => {
    mockApi((call) =>
      call.path.endsWith('/preview')
        ? json({ ...OK_PREVIEW, unknown: ['{{foo}}', '{{ code }}'], missing: ['{{code}}'] })
        : undefined,
    )
    const { client } = renderPage()
    const preview = await screen.findByRole('region', { name: '实时预览' }, { timeout: 4000 })
    expect(await within(preview).findByText('缺少必填变量')).toBeInTheDocument()
    expect(within(preview).getByText('未知变量')).toBeInTheDocument()
    expect(within(preview).getByText('{{code}}')).toBeInTheDocument()
    expect(within(preview).getByText('{{foo}}')).toBeInTheDocument()
    expect(within(preview).getByText('{{ code }}')).toBeInTheDocument() // near miss, shown as written
    const frame = within(preview).getByTitle('邮件预览：【GG Bond】验证码')
    expect(frame.tagName).toBe('IFRAME')
    expect(frame).toHaveAttribute('sandbox', '')
    expect(frame).toHaveAttribute('srcdoc', OK_PREVIEW.html)
    const user = userEvent.setup()
    await user.click(within(preview).getByRole('radio', { name: '纯文本' }))
    expect(within(preview).getByText(OK_PREVIEW.text)).toBeInTheDocument()
    client.clear()
  })

  it('maps errors.subject / errors.body of a failed save onto the fields', async () => {
    const calls = mockApi((call) =>
      call.method === 'PUT'
        ? json(
            {
              code: 'validation_failed',
              message: 'validation failed',
              errors: {
                subject: ['unknown placeholders: {{foo}}'],
                body: ['missing required placeholders: {{code}}'],
              },
            },
            400,
          )
        : undefined,
    )
    const { client } = renderPage()
    const subject = await subjectBox()
    const user = userEvent.setup()
    await user.clear(subject)
    await user.type(subject, '验证码 {{{{foo}}')
    await user.click(screen.getByRole('button', { name: '保存' }))

    expect(await screen.findByText('unknown placeholders: {{foo}}')).toBeInTheDocument()
    expect(screen.getByText('missing required placeholders: {{code}}')).toBeInTheDocument()
    expect(subject).toHaveAttribute('aria-invalid', 'true')
    expect(bodyBox()).toHaveAttribute('aria-invalid', 'true')
    const put = calls.find((c) => c.method === 'PUT')
    expect(put?.path).toBe('/admin/mail-templates/verify_code')
    expect(put?.body).toEqual({ locale: 'zh-CN', subject: '验证码 {{foo}}', body: ZH.body })
    client.clear()
  })

  it('asks before restoring the default and then calls DELETE', async () => {
    let restored = false
    const calls = mockApi((call) => {
      if (call.method === 'DELETE') {
        restored = true
        return json(true)
      }
      if (!restored || call.method !== 'GET') return undefined
      // After the reset the server answers with the built-in template (refetch after invalidation).
      if (call.path === '/admin/mail-templates')
        return json(
          SUMMARIES.map((s) => ({
            ...s,
            locales: { ...s.locales, 'zh-CN': { custom: false, updated_at: null } },
          })),
        )
      if (call.query.get('locale') === 'zh-CN')
        return json({ ...ZH, ...DEFAULT, custom: false, updated_at: null })
      return undefined
    })
    const { client } = renderPage()
    await subjectBox()
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: '恢复默认' }))
    const dialog = await screen.findByRole('alertdialog', { name: '将“邮箱验证码”恢复为默认模板？' })
    expect(calls.some((c) => c.method === 'DELETE')).toBe(false)
    await user.click(within(dialog).getByRole('button', { name: '恢复默认' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE')).toBe(true))
    const del = calls.find((c) => c.method === 'DELETE')
    expect(del?.path).toBe('/admin/mail-templates/verify_code')
    expect(del?.query.get('locale')).toBe('zh-CN')
    expect(await subjectBox()).toHaveValue(DEFAULT.subject)
    expect(screen.getByRole('button', { name: '恢复默认' })).toBeDisabled()
    client.clear()
  })

  it('loads the other locale, asking first when there are unsaved changes', async () => {
    const calls = mockApi()
    const { client } = renderPage()
    const subject = await subjectBox()
    const user = userEvent.setup()
    await user.type(subject, '!')
    await user.click(screen.getByRole('radio', { name: 'English' }))
    const dialog = await screen.findByRole('alertdialog', { name: '放弃未保存的更改？' })
    await user.click(within(dialog).getByRole('button', { name: '取消' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(screen.getByRole('radio', { name: /简体中文/ })).toBeChecked()
    expect(subject).toHaveValue(`${ZH.subject}!`)

    await user.click(screen.getByRole('radio', { name: 'English' }))
    await user.click(
      within(await screen.findByRole('alertdialog', { name: '放弃未保存的更改？' })).getByRole('button', {
        name: '放弃更改',
      }),
    )
    await waitFor(async () => expect(await subjectBox()).toHaveValue(EN.subject))
    expect(
      calls.some((c) => c.path === '/admin/mail-templates/verify_code' && c.query.get('locale') === 'en'),
    ).toBe(true)
    expect(screen.getByRole('radio', { name: 'English' })).toBeChecked()
    client.clear()
  })
})
