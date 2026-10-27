/**
 * Dev-only mock of the admin mail-template endpoints (docs/API.md "Mail templates"), plugged into
 * `server.ts` (`/admin/mail-templates/*`). In-memory overrides per template and locale; the preview is a
 * tiny Markdown subset rendered inside a layout that mimics `internal/mail/templates/layout.html`, with the
 * placeholders replaced by sample values. Never part of the bundle.
 *
 *   test mail to an address on fail.example.com → 400 with a driver error in `errors.email`
 */
import type { ServerResponse } from 'node:http'

type Json = Record<string, unknown>
type Locale = 'zh-CN' | 'en'
const LOCALES: readonly Locale[] = ['zh-CN', 'en']

interface Variable {
  name: string
  required: boolean
}
interface Content {
  subject: string
  body: string
}
interface Builtin {
  name: string
  variables: Variable[]
  defaults: Record<Locale, Content>
}

const v = (name: string, required = false): Variable => ({ name, required })
const COMMON = [v('siteName'), v('siteUrl'), v('year')]

const BUILTIN: Builtin[] = [
  {
    name: 'verify_code',
    variables: [v('code', true), v('minutes'), ...COMMON],
    defaults: {
      'zh-CN': {
        subject: '【{{siteName}}】邮箱验证码',
        body: '## 邮箱验证码\n\n您正在进行邮箱验证，验证码如下，{{minutes}} 分钟内有效：\n\n# {{code}}\n\n如果这不是您本人的操作，请忽略此邮件。',
      },
      en: {
        subject: '{{siteName}} verification code',
        body: '## Verify your email\n\nUse the code below to verify your email. It expires in {{minutes}} minutes.\n\n# {{code}}\n\nIf you did not request this, you can safely ignore this email.',
      },
    },
  },
  {
    name: 'remind_expire',
    variables: [v('expiredAt', true), v('url'), ...COMMON],
    defaults: {
      'zh-CN': {
        subject: '【{{siteName}}】您的服务即将到期',
        body: '## 服务即将到期\n\n您的服务将于 **{{expiredAt}}** 到期。为避免服务中断，请及时续费。\n\n[立即续费]({{url}})',
      },
      en: {
        subject: 'Your {{siteName}} subscription expires soon',
        body: '## Subscription expiring soon\n\nYour subscription expires on **{{expiredAt}}**. Renew now to avoid interruption.\n\n[Renew now]({{url}})',
      },
    },
  },
  {
    name: 'remind_traffic',
    variables: [v('percent', true), v('used'), v('total'), v('url'), ...COMMON],
    defaults: {
      'zh-CN': {
        subject: '【{{siteName}}】流量使用提醒',
        body: '## 流量即将用尽\n\n本周期您已使用 **{{percent}}%** 的流量（{{used}} / {{total}}）。\n\n[打开面板]({{url}})',
      },
      en: {
        subject: '{{siteName}} data usage alert',
        body: "## You're running low on data\n\nYou have used **{{percent}}%** of your data this cycle ({{used}} of {{total}}).\n\n[Open dashboard]({{url}})",
      },
    },
  },
  {
    name: 'order_paid',
    variables: [v('tradeNo', true), v('amount', true), v('planName'), v('period'), v('url'), ...COMMON],
    defaults: {
      'zh-CN': {
        subject: '【{{siteName}}】订单支付成功',
        body: '## 支付成功\n\n感谢您的购买，订单已处理完成。\n\n- 订单号：{{tradeNo}}\n- 套餐：{{planName}}\n- 周期：{{period}}\n- 金额：**{{amount}}**\n\n[查看订单]({{url}})',
      },
      en: {
        subject: '{{siteName}} payment received',
        body: '## Payment received\n\nThank you for your purchase. Your order is complete.\n\n- Order number: {{tradeNo}}\n- Plan: {{planName}}\n- Period: {{period}}\n- Amount: **{{amount}}**\n\n[View order]({{url}})',
      },
    },
  },
  {
    name: 'ticket_reply',
    variables: [v('ticketSubject', true), v('message'), v('url'), ...COMMON],
    defaults: {
      'zh-CN': {
        subject: '【{{siteName}}】您的工单有新回复',
        body: '## 工单有新回复\n\n您的工单「{{ticketSubject}}」收到了新的回复：\n\n> {{message}}\n\n[查看工单]({{url}})',
      },
      en: {
        subject: 'New reply to your {{siteName}} ticket',
        body: 'Your ticket "{{ticketSubject}}" received a new reply:\n\n> {{message}}\n\n[View ticket]({{url}})',
      },
    },
  },
  {
    name: 'subscription_reset',
    variables: [v('url'), ...COMMON],
    defaults: {
      'zh-CN': {
        subject: '【{{siteName}}】系统已重置您的订阅信息',
        body: '# 订阅信息已重置\n\n系统已重置您的订阅链接和 UUID，旧的订阅链接已失效。\n\n您也已在所有设备上退出登录。\n\n请重新登录网站，在“订阅”页面复制新的订阅链接，并重新导入到您的客户端中。\n\n[前往订阅页面]({{url}})',
      },
      en: {
        subject: 'Your {{siteName}} subscription was reset by the administrator',
        body: '# Your subscription was reset\n\nThe administrator reset your subscription link and UUID, so the old link no longer works.\n\nYou have also been signed out on all devices.\n\nSign in to the website again, copy the new subscription link from the Subscription page and import it into your clients.\n\n[Open subscription page]({{url}})',
      },
    },
  },
  {
    name: 'notify',
    variables: [v('content', true), v('title'), v('url'), ...COMMON],
    defaults: {
      'zh-CN': { subject: '{{title}}', body: '## {{title}}\n\n{{content}}' },
      en: { subject: '{{title}}', body: '## {{title}}\n\n{{content}}' },
    },
  },
]

const SAMPLE: Record<Locale, Record<string, string>> = {
  'zh-CN': {
    siteName: 'GG Bond',
    siteUrl: 'https://panel.example.com',
    year: String(new Date().getFullYear()),
    code: '428913',
    minutes: '10',
    expiredAt: '2026-11-01 20:00',
    url: 'https://panel.example.com/dashboard',
    percent: '86',
    used: '86.0 GB',
    total: '100.0 GB',
    tradeNo: '2026102312000012345',
    amount: '¥25.00',
    planName: 'Pro',
    period: '月付',
    ticketSubject: '节点连接超时',
    message: '您好，香港节点已恢复，请更新订阅后重试。\n如仍有问题请回复本工单。',
    content: '本周六 02:00–04:00 进行节点维护，期间部分节点不可用。\n感谢理解。',
    title: '维护通知',
  },
  en: {
    siteName: 'GG Bond',
    siteUrl: 'https://panel.example.com',
    year: String(new Date().getFullYear()),
    code: '428913',
    minutes: '10',
    expiredAt: '2026-11-01 20:00',
    url: 'https://panel.example.com/dashboard',
    percent: '86',
    used: '86.0 GB',
    total: '100.0 GB',
    tradeNo: '2026102312000012345',
    amount: '¥25.00',
    planName: 'Pro',
    period: 'Monthly',
    ticketSubject: 'Node connection timeout',
    message:
      'Hi, the Hong Kong node is back. Please update your subscription and try again.\nReply here if it still fails.',
    content:
      'Node maintenance this Saturday 02:00–04:00; some nodes will be unavailable.\nThanks for your patience.',
    title: 'Maintenance notice',
  },
}

const overrides = new Map<string, Content & { updated_at: string }>()
// One customised template so the list shows both states.
overrides.set('verify_code:zh-CN', {
  subject: '【{{siteName}}】您的验证码是 {{code}}',
  body: '## 欢迎加入 {{siteName}}\n\n您的邮箱验证码：\n\n# {{code}}\n\n验证码 {{minutes}} 分钟内有效，请勿告诉他人。',
  updated_at: new Date(Date.now() - 3 * 86_400_000).toISOString().replace(/\.\d{3}Z$/, 'Z'),
})

const PLACEHOLDER = /\{\{([A-Za-z0-9_]+)\}\}/g
const names = (s: string) => [...s.matchAll(PLACEHOLDER)].map((m) => m[1])
const esc = (s: string) =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')

function detail(b: Builtin, locale: Locale): Json {
  const o = overrides.get(`${b.name}:${locale}`)
  const def = b.defaults[locale]
  return {
    name: b.name,
    locale,
    custom: Boolean(o),
    subject: o?.subject ?? def.subject,
    body: o?.body ?? def.body,
    updated_at: o?.updated_at ?? null,
    default: { ...def },
  }
}

function summary(b: Builtin): Json {
  return {
    name: b.name,
    variables: b.variables,
    locales: Object.fromEntries(
      LOCALES.map((l) => {
        const o = overrides.get(`${b.name}:${l}`)
        return [l, { custom: Boolean(o), updated_at: o?.updated_at ?? null }]
      }),
    ),
  }
}

/** A small Markdown subset (headings, lists, quotes, bold, links) → inline-styled HTML. */
function markdown(src: string, values: Record<string, string>, known: Set<string>): string {
  const inline = (line: string) =>
    esc(line)
      .replace(PLACEHOLDER, (m, n: string) =>
        known.has(n) ? esc(values[n] ?? '').replace(/\n/g, '<br>') : m,
      )
      .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
      .replace(
        /\[([^\]]+)\]\(([^)]+)\)/g,
        '<a href="$2" style="color:#007AFF;font-weight:600;text-decoration:none;">$1</a>',
      )
  const out: string[] = []
  for (const block of src.replace(/\r\n/g, '\n').split(/\n{2,}/)) {
    const lines = block.split('\n')
    const first = lines[0] ?? ''
    if (/^# /.test(first))
      out.push(
        `<p style="margin:8px 0 16px;font-size:34px;line-height:40px;font-weight:700;letter-spacing:6px;color:#1C1C1E;">${inline(first.slice(2))}</p>`,
      )
    else if (/^## /.test(first))
      out.push(
        `<h1 style="margin:0 0 16px;font-size:22px;line-height:28px;font-weight:700;color:#1C1C1E;">${inline(first.slice(3))}</h1>`,
      )
    else if (lines.every((l) => /^- /.test(l)))
      out.push(
        `<ul style="margin:0 0 16px;padding-left:20px;font-size:16px;line-height:24px;color:#3C3C43;">${lines.map((l) => `<li>${inline(l.slice(2))}</li>`).join('')}</ul>`,
      )
    else if (lines.every((l) => /^> ?/.test(l)))
      out.push(
        `<blockquote style="margin:0 0 16px;padding:12px 16px;border-radius:12px;background:#F2F2F7;font-size:15px;line-height:22px;color:#3C3C43;">${lines.map((l) => inline(l.replace(/^> ?/, ''))).join('<br>')}</blockquote>`,
      )
    else
      out.push(
        `<p style="margin:0 0 16px;font-size:16px;line-height:24px;color:#3C3C43;">${lines.map(inline).join('<br>')}</p>`,
      )
  }
  return out.join('\n')
}

function render(b: Builtin, d: { locale: Locale; subject: string; body: string }): Json {
  const values = SAMPLE[d.locale]
  const known = new Set(b.variables.map((x) => x.name))
  const used = [...names(d.subject), ...names(d.body)]
  const unknown = [...new Set(used.filter((n) => !known.has(n)))].map((n) => `{{${n}}}`)
  const inBody = new Set(names(d.body))
  const missing = b.variables.filter((x) => x.required && !inBody.has(x.name)).map((x) => `{{${x.name}}}`)
  const fill = (s: string) => s.replace(PLACEHOLDER, (m, n: string) => (known.has(n) ? (values[n] ?? '') : m))
  const subject = fill(d.subject).replace(/\s+/g, ' ').trim()
  const html = `<!doctype html><html lang="${d.locale}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>${esc(subject)}</title></head>
<body style="margin:0;padding:0;background:#F2F2F7;font-family:-apple-system,BlinkMacSystemFont,'SF Pro Text','PingFang SC','Helvetica Neue',Helvetica,Arial,sans-serif;color:#1C1C1E;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#F2F2F7;"><tr><td align="center" style="padding:40px 16px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:560px;">
<tr><td align="center" style="padding:0 8px 24px;"><span style="font-size:22px;line-height:28px;font-weight:700;color:#1C1C1E;">${esc(values.siteName ?? '')}</span></td></tr>
<tr><td style="background:#FFFFFF;border-radius:18px;padding:36px 32px;">${markdown(d.body, values, known)}</td></tr>
<tr><td align="center" style="padding:24px 8px 0;font-size:12px;line-height:18px;color:#8E8E93;">${d.locale === 'zh-CN' ? '此邮件由系统自动发送，请勿直接回复。' : 'This email was sent automatically. Please do not reply.'}<br>${esc(values.siteName ?? '')} &middot; &copy; ${values.year ?? ''}</td></tr>
</table></td></tr></table></body></html>`
  const text = fill(d.body)
    .replace(/^#{1,6} /gm, '')
    .replace(/\*\*(.+?)\*\*/g, '$1')
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '$1 ($2)')
  return { subject, html, text, unknown, missing }
}

type Errors = Record<string, string[]>

/** PUT / test validation (docs/API.md "Mail templates" → Validation). */
function validate(
  b: Builtin,
  d: Json,
): { errors: Errors; draft?: { locale: Locale; subject: string; body: string } } {
  const errors: Errors = {}
  const locale = d.locale
  if (locale !== 'zh-CN' && locale !== 'en') errors.locale = ['must be zh-CN or en']
  const subject = typeof d.subject === 'string' ? d.subject.trim() : ''
  const body = typeof d.body === 'string' ? d.body : ''
  if (!subject) errors.subject = ['is required']
  else if ([...subject].length > 255) errors.subject = ['must be at most 255 characters']
  else if (/[\r\n]/.test(subject)) errors.subject = ['must be a single line']
  if (!body.trim()) errors.body = ['is required']
  else if (Buffer.byteLength(body) > 65535) errors.body = ['must be at most 65535 bytes']
  const known = new Set(b.variables.map((x) => x.name))
  for (const [field, text] of [
    ['subject', subject],
    ['body', body],
  ] as const) {
    const unknown = [...new Set(names(text).filter((n) => !known.has(n)))]
    if (unknown.length && !errors[field])
      errors[field] = [`unknown placeholders: ${unknown.map((n) => `{{${n}}}`).join(', ')}`]
  }
  const inBody = new Set(names(body))
  const missing = b.variables.filter((x) => x.required && !inBody.has(x.name))
  if (missing.length && !errors.body)
    errors.body = [`missing required placeholders: ${missing.map((x) => `{{${x.name}}}`).join(', ')}`]
  if (Object.keys(errors).length) return { errors }
  return { errors, draft: { locale: locale as Locale, subject, body } }
}

const now = () => new Date().toISOString().replace(/\.\d{3}Z$/, 'Z')

/**
 * Handles `/admin/mail-templates[/:name[/preview|test]]`. `rest` = path segments after `mail-templates`.
 * Returns false when the route is not one of these endpoints.
 */
export function handleMailTemplates(
  method: string,
  rest: readonly string[],
  query: URLSearchParams,
  body: Json,
  res: ServerResponse,
): boolean {
  const send = (status: number, payload: unknown) => {
    res.statusCode = status
    res.setHeader('Content-Type', 'application/json; charset=utf-8')
    res.setHeader('Cache-Control', 'no-store')
    res.end(JSON.stringify(payload))
    return true
  }
  const ok = (data: unknown) => send(200, { data })
  const invalid = (errors: Errors, message = 'validation failed') =>
    send(400, { code: 'validation_failed', message, errors })

  const [name, action] = rest
  if (!name) return method === 'GET' ? ok(BUILTIN.map(summary)) : false
  const b = BUILTIN.find((x) => x.name === decodeURIComponent(name))
  if (!b) return send(404, { code: 'not_found', message: 'unknown mail template' })

  if (!action) {
    const locale = (method === 'PUT' ? body.locale : query.get('locale')) as Locale
    if (method === 'GET' || method === 'DELETE') {
      if (!LOCALES.includes(locale)) return invalid({ locale: ['must be zh-CN or en'] })
      if (method === 'DELETE') {
        overrides.delete(`${b.name}:${locale}`)
        return ok(true)
      }
      return ok(detail(b, locale))
    }
    if (method === 'PUT') {
      const { errors, draft } = validate(b, body)
      if (!draft) return invalid(errors)
      overrides.set(`${b.name}:${draft.locale}`, {
        subject: draft.subject,
        body: draft.body,
        updated_at: now(),
      })
      return ok(detail(b, draft.locale))
    }
    return false
  }

  if (action === 'preview' && method === 'POST') {
    const locale = body.locale as Locale
    if (!LOCALES.includes(locale)) return invalid({ locale: ['must be zh-CN or en'] })
    const subject = typeof body.subject === 'string' ? body.subject : ''
    const text = typeof body.body === 'string' ? body.body : ''
    if (Buffer.byteLength(text) > 65535) return invalid({ body: ['must be at most 65535 bytes'] })
    return ok(render(b, { locale, subject, body: text }))
  }

  if (action === 'test' && method === 'POST') {
    const { errors, draft } = validate(b, body)
    const email = typeof body.email === 'string' ? body.email.trim() : ''
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) errors.email = ['invalid email']
    if (!draft || errors.email) return invalid(errors)
    if (email.endsWith('@fail.example.com'))
      return invalid(
        { email: ['dial tcp 192.0.2.10:465: connect: connection refused'] },
        'dial tcp 192.0.2.10:465: connect: connection refused',
      )
    return ok(true)
  }
  return false
}
