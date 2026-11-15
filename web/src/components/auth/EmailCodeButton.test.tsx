import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { LiveRegion } from '@/components/feedback/ToastMount'
import { loadNamespaces } from '@/i18n'
import type * as Passport from '@/lib/api/passport'
import { useUiStore } from '@/stores/ui'
import { EmailCodeButton } from './EmailCodeButton'

const sendEmailCode = vi.hoisted(() => vi.fn(() => Promise.resolve(true)))
vi.mock('@/lib/api/passport', async (original) => ({
  ...(await original<typeof Passport>()),
  sendEmailCode,
}))

describe('<EmailCodeButton/>', () => {
  beforeAll(async () => {
    useUiStore.setState({ locale: 'en' })
    await loadNamespaces(['auth'], 'en')
  })
  beforeEach(() => {
    sendEmailCode.mockClear()
    sessionStorage.clear()
    localStorage.clear()
  })

  it('keeps focus through the cooldown and says that the code was sent', async () => {
    const onResult = vi.fn()
    const user = userEvent.setup()
    render(
      <>
        <LiveRegion />
        <EmailCodeButton purpose="test" getEmail={() => 'a@example.com'} onResult={onResult} />
      </>,
    )
    const button = screen.getByRole('button', { name: 'Send code' })
    button.focus()
    await user.keyboard('{Enter}')

    await waitFor(() => expect(button).toHaveAccessibleName(/Resend in \d+s/))
    // `disabled` would have dropped focus to <body>; `aria-disabled` keeps the user where they were.
    expect(button).toHaveAttribute('aria-disabled', 'true')
    expect(button).not.toBeDisabled()
    expect(button).toHaveFocus()
    expect(onResult).toHaveBeenCalledExactlyOnceWith({ ok: true, email: 'a@example.com' })
    expect(document.getElementById('gg-live')).toHaveTextContent('Code sent. Check your inbox.')

    // Still unavailable: another press sends nothing.
    await user.keyboard('{Enter}')
    await user.click(button)
    expect(sendEmailCode).toHaveBeenCalledTimes(1)
  })
})
