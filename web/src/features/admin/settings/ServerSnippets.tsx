import { useState } from 'react'
import { CopyButton } from '@/components/CopyButton'
import { TitleWithHelp } from '@/components/HelpTip'
import { SegmentedTabs } from '@/components/SegmentedControl'
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
  const [tab, setTab] = useState<(typeof tabs)[number]['id']>('xrayr')
  return (
    <section aria-labelledby="server-snippets" className="flex flex-col gap-3">
      <TitleWithHelp help={t('server.snippetsHint')} subject={t('server.snippetsTitle')}>
        <h3 id="server-snippets" className="text-headline text-label">
          {t('server.snippetsTitle')}
        </h3>
      </TitleWithHelp>
      <SegmentedTabs value={tab} onChange={setTab} className="flex flex-col gap-3">
        <SegmentedTabs.List aria-label={t('server.snippetsTitle')} size="sm" items={tabs} />
        {tabs.map((item) => (
          <SegmentedTabs.Panel key={item.id} id={item.id}>
            <div className="relative">
              {/* Focusable so keyboard users can scroll long lines (axe scrollable-region-focusable). */}
              <pre
                tabIndex={0}
                className="overflow-x-auto rounded-[12px] bg-fill-quaternary p-4 pe-12 font-mono text-footnote leading-5 text-label focus-visible:ring-2 focus-visible:ring-focus focus-visible:outline-none"
              >
                <code>{shown[item.id]}</code>
              </pre>
              <div className="absolute end-2 top-2">
                <CopyButton value={real[item.id]} variant="ghost" />
              </div>
            </div>
          </SegmentedTabs.Panel>
        ))}
      </SegmentedTabs>
    </section>
  )
}
