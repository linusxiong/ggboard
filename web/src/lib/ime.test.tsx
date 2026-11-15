import { Input, TextField } from '@heroui/react'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { isImeEnter } from './ime'

/** The two event shapes the app passes in: React Aria's wrapped event (HeroUI fields) and React's own. */
function Fields({ onAria, onNative }: { onAria: () => void; onNative: () => void }) {
  return (
    <>
      <TextField
        aria-label="aria"
        onKeyDown={(e) => {
          if (e.key === 'Enter' && !isImeEnter(e)) onAria()
        }}
      >
        <Input />
      </TextField>
      <input
        aria-label="native"
        onKeyDown={(e) => {
          if (e.key === 'Enter' && !isImeEnter(e)) onNative()
        }}
      />
    </>
  )
}

describe('isImeEnter', () => {
  it.each([
    ['a plain Enter acts', { keyCode: 13 }, 1],
    ['an Enter that picks an IME candidate does not', { keyCode: 229, isComposing: true }, 0],
    // WebKit bug 165004: the composition has already ended when the keydown arrives.
    ['Safari, where only keyCode 229 tells', { keyCode: 229, isComposing: false }, 0],
  ])('%s', (_name, init, calls) => {
    const onAria = vi.fn()
    const onNative = vi.fn()
    render(<Fields onAria={onAria} onNative={onNative} />)
    fireEvent.keyDown(screen.getByLabelText('aria'), { key: 'Enter', ...init })
    fireEvent.keyDown(screen.getByLabelText('native'), { key: 'Enter', ...init })
    expect(onAria).toHaveBeenCalledTimes(calls)
    expect(onNative).toHaveBeenCalledTimes(calls)
  })
})
