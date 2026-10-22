import { Tabs } from '@heroui/react'
import { CopyButton } from '@/components/CopyButton'
import { useT } from '@/i18n'
import { buildSnippets } from './snippets'

interface ServerSnippetsProps {
  appUrl: string
  token: string
  /** Show the real token in the snippets (copy always uses it). */
  revealed: boolean
}

const MASK = '••••••••••••••••'

export function ServerSnippets({ appUrl, token, revealed }: ServerSnippetsProps) {
  const t = useT('admin-settings')
  const real = buildSnippets(appUrl, token)
  const shown = revealed || !token ? real : buildSnippets(appUrl, MASK)
  const tabs = [
    { id: 'xrayr', label: 'XrayR' },
    { id: 'v2bx', label: 'V2bX' },
    { id: 'soga', label: 'soga' },
  ] as const
  return (
    <section aria-labelledby="server-snippets" className="flex flex-col gap-3">
      <div>
        <h3 id="server-snippets" className="text-headline text-label">
          {t('server.snippetsTitle')}
        </h3>
        <p className="mt-1 text-footnote text-label-secondary">{t('server.snippetsHint')}</p>
      </div>
      <Tabs variant="secondary">
        <Tabs.ListContainer>
          <Tabs.List aria-label={t('server.snippetsTitle')}>
            {tabs.map((tab) => (
              <Tabs.Tab key={tab.id} id={tab.id}>
                {tab.label}
                <Tabs.Indicator />
              </Tabs.Tab>
            ))}
          </Tabs.List>
        </Tabs.ListContainer>
        {tabs.map((tab) => (
          <Tabs.Panel key={tab.id} id={tab.id} className="pt-3">
            <div className="relative">
              {/* Focusable so keyboard users can scroll long lines (axe scrollable-region-focusable). */}
              <pre
                tabIndex={0}
                className="overflow-x-auto rounded-[12px] bg-fill-quaternary p-4 pe-12 font-mono text-footnote leading-5 text-label focus-visible:ring-2 focus-visible:ring-focus focus-visible:outline-none"
              >
                <code>{shown[tab.id]}</code>
              </pre>
              <div className="absolute end-2 top-2">
                <CopyButton value={real[tab.id]} variant="ghost" />
              </div>
            </div>
          </Tabs.Panel>
        ))}
      </Tabs>
    </section>
  )
}
