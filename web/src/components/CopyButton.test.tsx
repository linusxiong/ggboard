import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { LiveRegion } from '@/components/feedback/ToastMount'
import { loadNamespaces } from '@/i18n'
import { toastStore } from '@/lib/toast'
import { useUiStore } from '@/stores/ui'
import { CopyButton } from './CopyButton'

describe('<CopyButton/>', () => {
  beforeAll(async () => {
    useUiStore.setState({ locale: 'en' })
    await loadNamespaces(['common'], 'en')
  })
  afterEach(() => {
    act(() => toastStore.clear())
  })

  it('keeps its name and announces the copy itself when it shows no toast', async () => {
    const user = userEvent.setup() // installs a clipboard stub on `navigator`
    const writeText = vi.spyOn(navigator.clipboard, 'writeText')
    render(
      <>
        <LiveRegion />
        <CopyButton value="GG-2026" notify={false} label="Copy code GG-2026" />
        <CopyButton value="other" notify={false} />
      </>,
    )
    await user.click(screen.getByRole('button', { name: 'Copy code GG-2026' }))

    expect(writeText).toHaveBeenCalledExactlyOnceWith('GG-2026')
    await waitFor(() => expect(document.getElementById('gg-live')).toHaveTextContent('Copied'))
    // The name says what the button does, before and after; an unnamed one falls back to "Copy".
    expect(screen.getByRole('button', { name: 'Copy code GG-2026' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Copy' })).toBeInTheDocument()
    expect(toastStore.getSnapshot()).toHaveLength(0)
  })
})
