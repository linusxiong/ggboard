import { render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { focusFirstInvalid } from './focus'

describe('focusFirstInvalid', () => {
  it('focuses the invalid control, not a helper button in front of it', async () => {
    render(
      <form aria-label="form">
        <div data-invalid="true">
          <button type="button" className="help-tip" aria-label="About name" />
          <input aria-label="Name" aria-invalid="true" />
        </div>
        <div data-invalid="true">
          <button type="button" aria-label="Plan" />
        </div>
        <button type="submit">Save</button>
      </form>,
    )
    const save = screen.getByRole('button', { name: 'Save' })
    save.focus()
    focusFirstInvalid(screen.getByRole('form'))
    await waitFor(() => expect(screen.getByLabelText('Name')).toHaveFocus())
  })

  it('still focuses a control that is a button (a select trigger), skipping chip remove buttons', async () => {
    render(
      <form aria-label="form">
        <div data-invalid="true">
          <button type="button" className="gg-tag__remove" aria-label="Remove a.com" />
          <button type="button" aria-label="Plan" />
        </div>
        <button type="submit">Save</button>
      </form>,
    )
    screen.getByRole('button', { name: 'Save' }).focus()
    focusFirstInvalid(screen.getByRole('form'))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Plan' })).toHaveFocus())
  })

  it('leaves focus alone when the user has moved it meanwhile', async () => {
    render(
      <form aria-label="form">
        <input aria-label="Name" aria-invalid="true" />
        <input aria-label="Other" />
        <button type="submit">Save</button>
      </form>,
    )
    screen.getByRole('button', { name: 'Save' }).focus()
    focusFirstInvalid(screen.getByRole('form'))
    screen.getByLabelText('Other').focus()
    await new Promise((resolve) => setTimeout(resolve, 60))
    expect(screen.getByLabelText('Other')).toHaveFocus()
  })
})
