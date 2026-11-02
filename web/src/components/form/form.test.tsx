import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/lib/api/client'
import { applyServerErrors, useAppForm } from './index'

function Demo({ onSubmit }: { onSubmit: (v: { email: string }) => void }) {
  const form = useAppForm({
    defaultValues: { email: '' },
    onSubmit: ({ value }) => onSubmit(value),
  })
  return (
    <form
      noValidate
      onSubmit={(e) => {
        e.preventDefault()
        void form.handleSubmit()
      }}
    >
      <form.AppField
        name="email"
        validators={{ onSubmit: ({ value }) => (value ? undefined : 'Email is required') }}
      >
        {(field) => <field.TextField label="Email" type="email" />}
      </form.AppField>
      <form.AppForm>
        <form.SubmitButton>Save</form.SubmitButton>
      </form.AppForm>
      <button
        type="button"
        onClick={() =>
          applyServerErrors(
            form,
            new ApiError(400, { code: 'validation_failed', message: 'x', errors: { email: ['taken'] } }),
          )
        }
      >
        server
      </button>
    </form>
  )
}

describe('useAppForm + HeroUI fields', () => {
  it('shows validator errors, submits valid values and maps server errors', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<Demo onSubmit={onSubmit} />)

    const input = screen.getByLabelText('Email')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText('Email is required')).toBeInTheDocument()
    expect(onSubmit).not.toHaveBeenCalled()

    await user.type(input, 'a@example.com')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(onSubmit).toHaveBeenCalledWith({ email: 'a@example.com' })

    await user.click(screen.getByRole('button', { name: 'server' }))
    expect(await screen.findByText('taken')).toBeInTheDocument()
    expect(input).toHaveAttribute('aria-invalid', 'true')
  })
})

describe('TextAreaField normalize', () => {
  function ListDemo({ onSubmit }: { onSubmit: (v: { list: string }) => void }) {
    const form = useAppForm({ defaultValues: { list: '' }, onSubmit: ({ value }) => onSubmit(value) })
    return (
      <form
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          void form.handleSubmit()
        }}
      >
        <form.AppField name="list">
          {(field) => (
            <field.TextAreaField
              label="List"
              normalize={(text) =>
                text
                  .split('\n')
                  .map((line) => line.replace(/^"|",?$/g, '').trim())
                  .filter(Boolean)
                  .join('\n')
              }
            />
          )}
        </form.AppField>
        <form.AppForm>
          <form.SubmitButton>Save</form.SubmitButton>
        </form.AppForm>
      </form>
    )
  }

  it('rewrites the text when the field loses focus and leaves clean text alone', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<ListDemo onSubmit={onSubmit} />)
    const area = screen.getByLabelText('List')
    await user.click(area)
    await user.paste('"geosite:bbc",\n"domain:rfa.org"\n')
    expect(area).toHaveValue('"geosite:bbc",\n"domain:rfa.org"\n')
    await user.tab()
    expect(area).toHaveValue('geosite:bbc\ndomain:rfa.org')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(onSubmit).toHaveBeenCalledWith({ list: 'geosite:bbc\ndomain:rfa.org' })
  })
})
