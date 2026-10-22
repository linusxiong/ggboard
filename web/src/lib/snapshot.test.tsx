import { act, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { describe, expect, it } from 'vitest'
import { carryInputs, restoreInputs } from './snapshot'

function Form() {
  const [email, setEmail] = useState('')
  return (
    <form>
      <input name="email" value={email} onChange={(e) => setEmail(e.currentTarget.value)} />
      <input name="password" type="password" defaultValue="" />
      <output>{email}</output>
    </form>
  )
}

describe('prerendered snapshot hand-over', () => {
  it('carries typed values and focus into the React tree that replaces the snapshot', () => {
    const root = document.createElement('div')
    document.body.append(root)
    // Static snapshot (what dist/login/index.html paints before the app loads); the visitor typed already.
    root.innerHTML = '<form><input name="email" value=""><input name="password" type="password"></form>'
    root.querySelector<HTMLInputElement>('[name=email]')!.value = 'a@example.com'
    root.querySelector<HTMLInputElement>('[name=password]')!.value = 'secret-pass'
    root.querySelector<HTMLInputElement>('[name=password]')!.focus()

    const carried = carryInputs(root)
    expect([...carried.values]).toEqual([
      ['email', 'a@example.com'],
      ['password', 'secret-pass'],
    ])
    expect(carried.focused).toBe('password')

    const app = createRoot(root)
    act(() => app.render(<Form />))
    act(() => restoreInputs(root, carried))

    expect(root.querySelector('output')?.textContent).toBe('a@example.com') // controlled state updated
    expect(root.querySelector<HTMLInputElement>('[name=password]')?.value).toBe('secret-pass')
    expect(document.activeElement?.getAttribute('name')).toBe('password')
    act(() => app.unmount())
    root.remove()
  })
})
