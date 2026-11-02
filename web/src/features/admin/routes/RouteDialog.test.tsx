import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeAll, describe, expect, it } from 'vitest'
import { loadNamespaces } from '@/i18n'
import { createQueryClient } from '@/lib/queryClient'
import { useUiStore } from '@/stores/ui'
import { RouteDialog } from './RouteDialog'

function renderDialog() {
  return render(
    <QueryClientProvider client={createQueryClient()}>
      <RouteDialog route={null} isOpen onOpenChange={() => undefined} />
    </QueryClientProvider>,
  )
}

describe('route dialog match box', () => {
  beforeAll(async () => {
    useUiStore.setState({ locale: 'zh-CN' })
    await loadNamespaces(['admin-nodes', 'common'])
  })

  it('tidies a pasted list and warns about prefixes only soga understands', async () => {
    const user = userEvent.setup()
    renderDialog()
    const match = await screen.findByRole('textbox', { name: /^匹配规则/ })
    expect(screen.queryByRole('status')).toBeNull()

    await user.click(match)
    await user.paste('(.*\\.)?doubleclick\\.net\n"regexp:.*\\.example\\.cn",')
    expect(screen.queryByRole('status')).toBeNull()

    await user.paste('\n"geosite:falungong",\n"domain:rfa.org",\n')
    const note = await screen.findByRole('status')
    expect(note).toHaveTextContent('geosite: / domain:')
    expect(note).toHaveTextContent('XrayR / V2bX')

    await user.tab()
    expect(match).toHaveValue('(.*\\.)?doubleclick\\.net\nregexp:.*\\.example\\.cn\ngeosite:falungong\ndomain:rfa.org')
  })
})
