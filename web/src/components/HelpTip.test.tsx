import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeAll, describe, expect, it } from 'vitest'
import { loadNamespaces } from '@/i18n'
import { useAppForm } from './form'
import { HelpTip } from './HelpTip'

function Fields() {
  const form = useAppForm({
    defaultValues: { name: '', size: 1, kind: 'a', on: false },
  })
  return (
    <form noValidate>
      <form.AppField name="name">
        {(field) => <field.TextField label="Name" isRequired description="Shown on the invoice." />}
      </form.AppField>
      <form.AppField name="size">
        {(field) => <field.NumberField label="Size" unit="GB" description="Per month." />}
      </form.AppField>
      <form.AppField name="kind">
        {(field) => (
          <field.SelectField
            label="Kind"
            description="Pick one."
            options={[
              { value: 'a', label: 'A' },
              { value: 'b', label: 'B' },
            ]}
          />
        )}
      </form.AppField>
      <form.AppField name="on">
        {(field) => <field.SwitchField label="Visible" description="Hidden when off." />}
      </form.AppField>
    </form>
  )
}

describe('HelpTip', () => {
  beforeAll(() => loadNamespaces(['common']))

  it('opens the explanation in a popover', async () => {
    render(<HelpTip subject="Category">Pick an existing category or type a new one.</HelpTip>)
    expect(screen.queryByText(/existing category/)).toBeNull()
    await userEvent.click(await screen.findByRole('button', { name: '关于「Category」的说明' }))
    expect(await screen.findByRole('dialog')).toHaveTextContent(
      'Pick an existing category or type a new one.',
    )
  })

  it('moves field descriptions behind the label ⓘ and keeps them as the accessible description', async () => {
    render(<Fields />)
    // Not on the page's first level, but still read out with the input.
    expect(await screen.findByRole('textbox', { name: 'Name' })).toHaveAccessibleDescription(
      'Shown on the invoice.',
    )
    expect(screen.getByText('Shown on the invoice.')).toHaveClass('sr-only')
    // Inside a NumberField / Select the ⓘ must not become a stepper or the select trigger.
    await userEvent.click(screen.getByRole('button', { name: '关于「Size」的说明' }))
    expect(await screen.findByRole('dialog')).toHaveTextContent('Per month.')
    await userEvent.keyboard('{Escape}')
    await userEvent.click(screen.getByRole('button', { name: '关于「Kind」的说明' }))
    expect(await screen.findByRole('dialog')).toHaveTextContent('Pick one.')
    expect(screen.queryByRole('listbox')).toBeNull()
    await userEvent.keyboard('{Escape}')
    // A switch is a <label>: its ⓘ sits outside it, so the switch's name stays its label.
    expect(screen.getByRole('switch', { name: 'Visible' })).not.toBeChecked()
    await userEvent.click(screen.getByRole('button', { name: '关于「Visible」的说明' }))
    expect(await screen.findByRole('dialog')).toHaveTextContent('Hidden when off.')
    await userEvent.keyboard('{Escape}')
    expect(screen.getByRole('switch', { name: 'Visible' })).not.toBeChecked()
  })
})
