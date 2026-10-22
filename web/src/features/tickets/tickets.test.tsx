import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it } from 'vitest'
import type { Ticket } from '@/lib/api/types'
import { bodyOf, callsTo, mockApi, renderApp, reply, resetApp } from '@/features/shop/testing'

const openTicket: Ticket = {
  id: 7,
  user_id: 1,
  subject: '节点很慢',
  level: 1,
  status: 0,
  reply_status: 1,
  last_reply_user_id: 99,
  created_at: '2026-10-21T10:00:00Z',
  updated_at: '2026-10-21T12:00:00Z',
}

describe('tickets', () => {
  afterEach(resetApp)

  it('explains the one-open-ticket rule on 409 and links to the open ticket', async () => {
    const fetchMock = mockApi({
      'GET /user/tickets': ({ url }) =>
        url.searchParams.get('status') === '0' ? [openTicket] : [openTicket],
      'POST /user/tickets': () =>
        reply(409, { code: 'conflict', message: 'you already have an open ticket' }),
    })
    renderApp('/tickets')
    const u = userEvent.setup()

    expect(await screen.findByRole('link', { name: '节点很慢' }, { timeout: 4000 })).toHaveAttribute(
      'href',
      '/tickets/7',
    )
    await u.click(screen.getByRole('button', { name: '新建工单' }))
    const dialog = await screen.findByRole('dialog', {}, { timeout: 4000 })

    // Client-side validation first.
    await u.click(within(dialog).getByRole('button', { name: '提交工单' }))
    expect(await within(dialog).findByText('请填写主题')).toBeInTheDocument()
    expect(callsTo(fetchMock, 'POST', '/user/tickets')).toHaveLength(0)

    await u.type(within(dialog).getByLabelText(/主题/), '无法连接')
    await u.type(within(dialog).getByLabelText(/问题描述/), '所有节点超时')
    await u.click(within(dialog).getByRole('button', { name: '提交工单' }))

    expect(await within(dialog).findByText('你还有一个未关闭的工单')).toBeInTheDocument()
    expect(within(dialog).getByRole('link', { name: '前往该工单' })).toHaveAttribute('href', '/tickets/7')
    expect(bodyOf(callsTo(fetchMock, 'POST', '/user/tickets')[0])).toEqual({
      subject: '无法连接',
      level: 1,
      message: '所有节点超时',
    })
  })

  it('shows the thread with my messages and staff replies, and sends a reply', async () => {
    let messages = [
      { id: 1, user_id: 1, is_me: true, message: '第一行\n第二行', created_at: '2026-10-21T10:00:00Z' },
      { id: 2, user_id: 99, is_me: false, message: '<b>已处理</b>', created_at: '2026-10-21T12:00:00Z' },
    ]
    const fetchMock = mockApi({
      'GET /user/tickets/7': () => ({ ...openTicket, messages }),
      'POST /user/tickets/7/reply': ({ body }) => {
        messages = [
          ...messages,
          {
            id: 3,
            user_id: 1,
            is_me: true,
            message: (body as { message: string }).message,
            created_at: '2026-10-22T00:00:00Z',
          },
        ]
        return { ...openTicket, reply_status: 0, messages }
      },
    })
    renderApp('/tickets/7')
    const u = userEvent.setup()

    const log = await screen.findByRole('log', { name: '消息记录' }, { timeout: 4000 })
    // Plain text: markup is shown literally, line breaks preserved.
    expect(within(log).getByText('<b>已处理</b>')).toBeInTheDocument()
    expect(within(log).getByText(/第一行/).textContent).toBe('第一行\n第二行')

    await u.type(screen.getByLabelText('回复内容'), '谢谢')
    await u.click(screen.getByRole('button', { name: '发送' }))
    await waitFor(() => expect(within(log).getByText('谢谢')).toBeInTheDocument())
    expect(callsTo(fetchMock, 'POST', '/user/tickets/7/reply')).toHaveLength(1)
  })
})
