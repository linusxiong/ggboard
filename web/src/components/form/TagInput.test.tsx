import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { TagInput, type TagInputProps } from './TagInput'
import { splitTagText } from './tags'

function Harness(props: Partial<TagInputProps> & { initial?: string[]; onValue?: (v: string[]) => void }) {
  const { initial = [], onValue, ...rest } = props
  const [value, setValue] = useState<string[]>(initial)
  return (
    <form onSubmit={(e) => e.preventDefault()}>
      <TagInput
        label="Domains"
        name="domains"
        value={value}
        onChange={(v) => {
          setValue(v)
          onValue?.(v)
        }}
        {...rest}
      />
    </form>
  )
}

const chips = () => screen.queryAllByRole('listitem').map((li) => li.textContent?.replace(/\s+/g, ' ').trim())

describe('splitTagText', () => {
  it('splits on ASCII / full-width commas, semicolons and newlines', () => {
    expect(splitTagText(' a.com, b.com；c.com\nd.com，e.com ;; ')).toEqual([
      'a.com',
      'b.com',
      'c.com',
      'd.com',
      'e.com',
    ])
  })
})

describe('<TagInput/>', () => {
  it('adds on Enter and on comma, ignores duplicates, labels the input', async () => {
    const user = userEvent.setup()
    render(<Harness />)
    const input = screen.getByRole('textbox', { name: 'Domains' })
    await user.type(input, 'gmail.com{Enter}')
    await user.type(input, 'outlook.com,')
    await user.type(input, 'gmail.com{Enter}')
    expect(chips()).toEqual(['gmail.com', 'outlook.com'])
    expect(input).toHaveValue('')
    expect(screen.getByRole('group', { name: 'Domains' })).toBeInTheDocument()
  })

  it('splits a pasted list into chips', async () => {
    const user = userEvent.setup()
    render(<Harness />)
    const input = screen.getByRole('textbox', { name: 'Domains' })
    await user.click(input)
    await user.paste('a.com, b.com\nc.com')
    expect(chips()).toEqual(['a.com', 'b.com', 'c.com'])
  })

  it('normalizes, validates and keeps the invalid item in the input with an error', async () => {
    const user = userEvent.setup()
    render(
      <Harness
        normalize={(s) => s.replace(/^@/, '').toLowerCase()}
        validate={(s) => (s.includes('.') ? undefined : 'Not a domain')}
      />,
    )
    const input = screen.getByRole('textbox', { name: 'Domains' })
    await user.type(input, '@Example.COM{Enter}')
    expect(chips()).toEqual(['example.com'])
    await user.type(input, 'nodot{Enter}')
    expect(screen.getByText('Not a domain')).toHaveAttribute('role', 'alert')
    expect(input).toHaveValue('nodot')
    expect(input).toHaveAttribute('aria-invalid', 'true')
    await user.type(input, '.org')
    expect(screen.queryByText('Not a domain')).not.toBeInTheDocument()
  })

  it('Backspace on an empty input selects, then removes the last chip; chips have remove buttons', async () => {
    const user = userEvent.setup()
    const onValue = vi.fn()
    render(<Harness initial={['a.com', 'b.com', 'c.com']} onValue={onValue} />)
    const input = screen.getByRole('textbox', { name: 'Domains' })
    await user.click(input)
    await user.keyboard('{Backspace}')
    expect(onValue).not.toHaveBeenCalled()
    await user.keyboard('{Backspace}')
    expect(onValue).toHaveBeenLastCalledWith(['a.com', 'b.com'])
    await user.click(screen.getByRole('button', { name: 'Remove a.com' }))
    expect(chips()).toEqual(['b.com'])
  })

  it('adds the draft on blur and lets Enter submit when the input is empty', async () => {
    const user = userEvent.setup()
    const submit = vi.fn((e: Event) => e.preventDefault())
    render(<Harness />)
    const input = screen.getByRole('textbox', { name: 'Domains' })
    input.closest('form')!.addEventListener('submit', submit)
    await user.type(input, 'x.com')
    fireEvent.blur(input)
    expect(chips()).toEqual(['x.com'])
    await user.type(input, '{Enter}')
    expect(submit).toHaveBeenCalledTimes(1)
  })

  it('suggestion chips add an item and disappear once added', async () => {
    const user = userEvent.setup()
    render(<Harness suggestions={['gmail.com', 'qq.com']} addLabel="Add" />)
    await user.click(screen.getByRole('button', { name: 'Add qq.com' }))
    expect(chips()).toEqual(['qq.com'])
    expect(screen.queryByRole('button', { name: 'Add qq.com' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Add gmail.com' })).toBeInTheDocument()
  })
})
