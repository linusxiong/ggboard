/**
 * Type-specific `protocol_settings` form for every node type (vmess, vless, shadowsocks,
 * shadowsocksr, trojan, hysteria 1/2, anytls, mieru, tuic). Edits go through `updateSetting`
 * (dependent resets); errors are keyed by path inside `protocol_settings`.
 */
import { Alert, Button } from '@heroui/react'
import { KeyRound, Shuffle } from 'lucide-react'
import { type ReactNode, useMemo } from 'react'
import { useT } from '@/i18n'
import type { NodeType, TransportNetwork } from '@/lib/api/types'
import { toast } from '@/lib/toast'
import {
  ANYTLS_DEFAULT_PADDING,
  compatNotes,
  FINGERPRINTS,
  generateRealityKeys,
  isSs2022,
  MIERU_MULTIPLEXING,
  NETWORKS,
  randomBase64Key,
  randomHex,
  type Settings,
  SS_CIPHERS,
  ss2022KeyBytes,
  SSR_CIPHERS,
  SSR_OBFS,
  SSR_PROTOCOLS,
  TUIC_CONGESTION,
  TUIC_UDP_RELAY,
  updateSetting,
  XHTTP_MODES,
} from './protocol'
import { ProtocolContext, type ProtocolContextValue, useSetting } from './protocol-context'
import { PJson, PList, PNumber, type POption, PSelect, PSwitch, PText } from './ProtocolFields'

export interface ProtocolFormProps {
  type: NodeType
  value: Settings
  onChange: (next: Settings) => void
  /** Messages keyed by path inside `protocol_settings` (server + client validation). */
  errors?: Record<string, string>
  /** Called with the edited path (lets the parent clear that path's error). */
  onEdit?: (path: string) => void
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <fieldset className="flex min-w-0 flex-col gap-4">
      <legend className="mb-3 text-headline text-label">{title}</legend>
      {children}
    </fieldset>
  )
}

const Grid = ({ children }: { children: ReactNode }) => (
  <div className="grid gap-4 sm:grid-cols-2">{children}</div>
)

const opts = (values: readonly string[]): POption[] => values.map((v) => ({ value: v, label: v }))

// ---------------------------------------------------------------- shared blocks

function NetworkFields() {
  const { settings, t, type } = useSetting('network')
  const network = (settings.network as TransportNetwork | undefined) ?? 'tcp'
  const networks = NETWORKS[type] ?? []
  const tp = (k: string) => t(`nodes.ps.${k}`)
  return (
    <Section title={tp('sections.transport')}>
      <PSelect
        path="network"
        label={tp('network')}
        fallback="tcp"
        options={networks.map((n) => ({ value: n, label: tp(`networks.${n}`) }))}
      />
      {network === 'tcp' && type !== 'trojan' ? (
        <PSelect
          path="network_settings.header.type"
          label={tp('tcpHeader')}
          fallback="none"
          options={[
            { value: 'none', label: tp('tcpHeaders.none') },
            { value: 'http', label: tp('tcpHeaders.http') },
          ]}
        />
      ) : null}
      {network === 'ws' ? (
        <Grid>
          <PText path="network_settings.path" label={tp('path')} placeholder="/ws" mono />
          <PText path="network_settings.headers.Host" label={tp('wsHost')} placeholder="cdn.example.com" />
        </Grid>
      ) : null}
      {network === 'grpc' ? (
        <PText path="network_settings.serviceName" label={tp('serviceName')} placeholder="grpc" mono />
      ) : null}
      {network === 'http' || network === 'h2' ? (
        <Grid>
          <PText path="network_settings.path" label={tp('path')} placeholder="/" mono />
          <PList
            path="network_settings.host"
            label={tp('httpHost')}
            placeholder="a.example.com, b.example.com"
          />
        </Grid>
      ) : null}
      {network === 'httpupgrade' ? (
        <Grid>
          <PText path="network_settings.path" label={tp('path')} placeholder="/up" mono />
          <PText path="network_settings.host" label={tp('host')} placeholder="cdn.example.com" />
        </Grid>
      ) : null}
      {network === 'xhttp' ? (
        <>
          <Grid>
            <PText path="network_settings.path" label={tp('path')} placeholder="/xh" mono />
            <PText path="network_settings.host" label={tp('host')} placeholder="cdn.example.com" />
          </Grid>
          <PSelect path="network_settings.mode" label={tp('xhttpMode')} options={opts(XHTTP_MODES)} />
          <PJson
            key={`extra-${network}`}
            path="network_settings.extra"
            label={tp('xhttpExtra')}
            description={tp('xhttpExtraHint')}
            invalidLabel={tp('validation.invalidJson')}
            rows={4}
          />
        </>
      ) : null}
      <details className="rounded-[12px] bg-fill-quaternary px-3 py-2">
        <summary className="cursor-pointer text-subhead text-label-secondary">{tp('networkJson')}</summary>
        <div className="pt-3">
          <PJson
            key={network}
            path="network_settings"
            label={tp('networkJson')}
            description={tp('networkJsonHint')}
            invalidLabel={tp('validation.invalidJson')}
          />
        </div>
      </details>
    </Section>
  )
}

function TlsBasics({ insecure = true }: { insecure?: boolean }) {
  const { t } = useSetting('tls_settings')
  const tp = (k: string) => t(`nodes.ps.${k}`)
  return (
    <>
      <PText path="tls_settings.server_name" label={tp('serverName')} placeholder="www.example.com" />
      {insecure ? (
        <PSwitch
          path="tls_settings.allow_insecure"
          label={tp('allowInsecure')}
          description={tp('allowInsecureHint')}
        />
      ) : null}
    </>
  )
}

/** REALITY key pair + short id inputs with generators (vless / anytls). */
function RealityKeys() {
  const { t, set: setTls, settings } = useSetting('tls_settings')
  const shortId = useSetting('tls_settings.short_id')
  const tp = (k: string) => t(`nodes.ps.${k}`)
  const generate = async () => {
    const keys = await generateRealityKeys()
    if (!keys) {
      toast.warning(tp('keysUnsupported'))
      return
    }
    const ts = (settings.tls_settings ?? {}) as Settings
    setTls({ ...ts, private_key: keys.privateKey, public_key: keys.publicKey })
    toast.success(tp('keysGenerated'))
  }
  return (
    <>
      <div className="flex justify-end">
        <Button size="sm" variant="secondary" onPress={() => void generate()}>
          <KeyRound className="size-4" aria-hidden="true" />
          {tp('generateKeys')}
        </Button>
      </div>
      <PText
        path="tls_settings.private_key"
        label={tp('privateKey')}
        mono
        isRequired
        description={tp('privateKeyHint')}
      />
      <PText
        path="tls_settings.public_key"
        label={tp('publicKey')}
        mono
        isRequired
        description={tp('publicKeyHint')}
      />
      <PText
        path="tls_settings.short_id"
        label={tp('shortId')}
        mono
        description={tp('shortIdHint')}
        end={
          <Button
            size="sm"
            variant="tertiary"
            isIconOnly
            aria-label={tp('generateShortId')}
            onPress={() => shortId.set(randomHex(4))}
          >
            <Shuffle className="size-4" aria-hidden="true" />
          </Button>
        }
      />
    </>
  )
}

// ---------------------------------------------------------------- per type

function VmessFields() {
  const { t, value: tls } = useSetting('tls')
  const tp = (k: string) => t(`nodes.ps.${k}`)
  return (
    <>
      <NetworkFields />
      <Section title={tp('sections.security')}>
        <PSelect
          path="tls"
          label={tp('tls')}
          options={[0, 1].map((v) => ({ value: v, label: tp(`tlsModes.${v}`) }))}
          description={tp('tlsLocalCert')}
        />
        {tls === 1 ? <TlsBasics /> : null}
      </Section>
    </>
  )
}

function VlessFields() {
  const { t, value: tls } = useSetting('tls')
  const tp = (k: string) => t(`nodes.ps.${k}`)
  return (
    <>
      <NetworkFields />
      <Section title={tp('sections.security')}>
        <Grid>
          <PSelect
            path="tls"
            label={tp('tls')}
            options={[0, 1, 2].map((v) => ({ value: v, label: tp(`tlsModes.${v}`) }))}
          />
          <PSelect
            path="flow"
            label={tp('flow')}
            options={[
              { value: '', label: tp('flows.none') },
              { value: 'xtls-rprx-vision', label: 'xtls-rprx-vision' },
            ]}
          />
        </Grid>
        {tls === 1 ? <TlsBasics /> : null}
        {tls === 2 ? (
          <>
            <Grid>
              <PText
                path="tls_settings.server_name"
                label={tp('serverName')}
                placeholder="www.example.org"
                isRequired
              />
              <PText
                path="tls_settings.dest"
                label={tp('realityDest')}
                placeholder="www.example.org:443"
                description={tp('realityDestHint')}
              />
            </Grid>
            <PSelect path="tls_settings.fingerprint" label={tp('fingerprint')} options={opts(FINGERPRINTS)} />
            <RealityKeys />
          </>
        ) : null}
      </Section>
      <Section title={tp('sections.encryption')}>
        <Grid>
          <PText
            path="encryption"
            label={tp('encryption')}
            placeholder="none"
            mono
            description={tp('encryptionHint')}
          />
          <PText path="decryption" label={tp('decryption')} mono description={tp('decryptionHint')} />
        </Grid>
      </Section>
    </>
  )
}

function ShadowsocksFields() {
  const { t, value: cipher, settings } = useSetting('cipher')
  const serverKey = useSetting('server_key')
  const tp = (k: string, p?: Record<string, string | number>) => t(`nodes.ps.${k}`, p)
  const obfs = settings.obfs ?? 'plain'
  const bytes = ss2022KeyBytes(cipher)
  return (
    <>
      <Section title={tp('sections.cipher')}>
        <PSelect path="cipher" label={tp('cipher')} options={opts(SS_CIPHERS)} />
        {isSs2022(cipher) ? (
          <PText
            path="server_key"
            label={tp('serverKey')}
            mono
            description={tp('serverKeyHint', { bytes })}
            end={
              <Button
                size="sm"
                variant="tertiary"
                isIconOnly
                aria-label={tp('generate')}
                onPress={() => serverKey.set(randomBase64Key(bytes))}
              >
                <Shuffle className="size-4" aria-hidden="true" />
              </Button>
            }
          />
        ) : null}
      </Section>
      <Section title={tp('sections.obfs')}>
        <PSelect
          path="obfs"
          label={tp('obfs')}
          options={['plain', 'http', 'tls'].map((v) => ({ value: v, label: tp(`obfsModes.${v}`) }))}
        />
        {obfs !== 'plain' ? (
          <Grid>
            <PText path="obfs_settings.host" label={tp('obfsHost')} placeholder="www.example.net" />
            <PText path="obfs_settings.path" label={tp('obfsPath')} placeholder="/" mono />
          </Grid>
        ) : null}
      </Section>
      <Section title={tp('sections.plugin')}>
        <Grid>
          <PText path="plugin" label={tp('plugin')} placeholder="obfs-local" description={tp('pluginHint')} />
          <PText
            path="plugin_opts"
            label={tp('pluginOpts')}
            placeholder="obfs=http;obfs-host=www.example.net"
            mono
          />
        </Grid>
      </Section>
    </>
  )
}

function ShadowsocksRFields() {
  const { t } = useSetting('cipher')
  const tp = (k: string) => t(`nodes.ps.${k}`)
  return (
    <Section title={tp('sections.cipher')}>
      <Grid>
        <PSelect path="cipher" label={tp('cipher')} options={opts(SSR_CIPHERS)} />
        <PText path="password" label={tp('ssrPassword')} mono description={tp('ssrPasswordHint')} />
        <PSelect path="protocol" label={tp('protocol')} options={opts(SSR_PROTOCOLS)} />
        <PText path="protocol_param" label={tp('protocolParam')} mono />
        <PSelect path="obfs" label={tp('obfs')} options={opts(SSR_OBFS)} />
        <PText path="obfs_param" label={tp('obfsParam')} mono />
      </Grid>
      <PSelect
        path="single_port_type"
        label={tp('singlePort')}
        options={['protocol', 'obfs'].map((v) => ({ value: v, label: tp(`singlePorts.${v}`) }))}
        description={tp('singlePortHint')}
      />
    </Section>
  )
}

function TrojanFields() {
  const { t } = useSetting('server_name')
  const tp = (k: string) => t(`nodes.ps.${k}`)
  return (
    <>
      <NetworkFields />
      <Section title={tp('sections.security')}>
        <p className="-mt-2 text-footnote text-label-secondary">{tp('trojanTls')}</p>
        <PText path="server_name" label={tp('serverName')} placeholder="www.example.com" />
        <PSwitch path="allow_insecure" label={tp('allowInsecure')} description={tp('allowInsecureHint')} />
      </Section>
    </>
  )
}

function HysteriaFields() {
  const { t, value: version, settings } = useSetting('version')
  const tp = (k: string) => t(`nodes.ps.${k}`)
  const obfs = settings.obfs ?? 'plain'
  return (
    <>
      <Section title={tp('sections.basics')}>
        <PSelect
          path="version"
          label={tp('version')}
          options={[2, 1].map((v) => ({ value: v, label: tp(`versions.${v}`) }))}
        />
        <Grid>
          <PNumber path="up_mbps" label={tp('upMbps')} description={tp('bandwidthHint')} />
          <PNumber path="down_mbps" label={tp('downMbps')} description={tp('bandwidthHint')} />
        </Grid>
        {version === 2 ? (
          <PSwitch
            path="ignore_client_bandwidth"
            label={tp('ignoreClientBandwidth')}
            description={tp('ignoreClientBandwidthHint')}
          />
        ) : null}
      </Section>
      <Section title={tp('sections.obfs')}>
        <PSelect
          path="obfs"
          label={tp('obfs')}
          options={[
            { value: 'plain', label: tp('hyObfs.plain') },
            { value: 'salamander', label: version === 1 ? tp('hyObfs.v1') : tp('hyObfs.salamander') },
          ]}
        />
        {obfs !== 'plain' ? <PText path="obfs_password" label={tp('obfsPassword')} mono isRequired /> : null}
      </Section>
      <Section title={tp('sections.tls')}>
        <Grid>
          <PText path="server_name" label={tp('serverName')} placeholder="www.example.com" />
          <PText path="alpn" label={tp('alpn')} placeholder="h3" mono />
        </Grid>
        <PSwitch path="allow_insecure" label={tp('allowInsecure')} description={tp('allowInsecureHint')} />
        <PNumber path="hop_interval" label={tp('hopInterval')} description={tp('hopIntervalHint')} />
      </Section>
    </>
  )
}

function AnyTlsFields() {
  const { t, value: tls } = useSetting('tls')
  const padding = useSetting('padding_scheme')
  const tp = (k: string) => t(`nodes.ps.${k}`)
  return (
    <>
      <Section title={tp('sections.security')}>
        <PSelect
          path="tls"
          label={tp('tls')}
          options={[1, 2].map((v) => ({ value: v, label: tp(`tlsModes.${v}`) }))}
        />
        <PText path="server_name" label={tp('serverName')} placeholder="www.example.com" />
        {tls === 1 ? (
          <PSwitch path="allow_insecure" label={tp('allowInsecure')} description={tp('allowInsecureHint')} />
        ) : null}
        {tls === 2 ? (
          <>
            <Grid>
              <PList
                path="tls_settings.server_names"
                label={tp('serverNames')}
                placeholder="www.example.org"
                description={tp('serverNamesHint')}
              />
              <PText
                path="tls_settings.dest"
                label={tp('realityDest')}
                placeholder="www.example.org:443"
                description={tp('realityDestHint')}
              />
            </Grid>
            <RealityKeys />
          </>
        ) : null}
      </Section>
      <Section title={tp('sections.padding')}>
        <PList
          path="padding_scheme"
          separator="line"
          rows={6}
          label={tp('paddingScheme')}
          description={tp('paddingHint')}
          placeholder={ANYTLS_DEFAULT_PADDING.slice(0, 3).join('\n')}
          end={
            <Button size="sm" variant="tertiary" onPress={() => padding.set([...ANYTLS_DEFAULT_PADDING])}>
              {tp('useDefaultPadding')}
            </Button>
          }
        />
      </Section>
    </>
  )
}

function MieruFields() {
  const { t } = useSetting('transport')
  const tp = (k: string) => t(`nodes.ps.${k}`)
  return (
    <Section title={tp('sections.basics')}>
      <Grid>
        <PSelect path="transport" label={tp('transport')} options={opts(['TCP', 'UDP'])} />
        <PSelect
          path="multiplexing"
          label={tp('multiplexing')}
          options={[
            { value: '', label: tp('multiplexingModes.default') },
            ...MIERU_MULTIPLEXING.map((v) => ({ value: v, label: tp(`multiplexingModes.${v}`) })),
          ]}
        />
      </Grid>
    </Section>
  )
}

function TuicFields() {
  const { t } = useSetting('congestion_control')
  const tp = (k: string) => t(`nodes.ps.${k}`)
  return (
    <>
      <Section title={tp('sections.basics')}>
        <Grid>
          <PSelect path="congestion_control" label={tp('congestion')} options={opts(TUIC_CONGESTION)} />
          <PSelect path="udp_relay_mode" label={tp('udpRelay')} options={opts(TUIC_UDP_RELAY)} />
        </Grid>
        <PSwitch path="zero_rtt_handshake" label={tp('zeroRtt')} />
      </Section>
      <Section title={tp('sections.tls')}>
        <Grid>
          <PText path="server_name" label={tp('serverName')} placeholder="www.example.com" />
          <PText path="alpn" label={tp('alpn')} placeholder="h3" mono description={tp('alpnHint')} />
        </Grid>
        <PSwitch path="allow_insecure" label={tp('allowInsecure')} description={tp('allowInsecureHint')} />
      </Section>
    </>
  )
}

const FIELDS: Record<NodeType, () => ReactNode> = {
  vmess: VmessFields,
  vless: VlessFields,
  shadowsocks: ShadowsocksFields,
  shadowsocksr: ShadowsocksRFields,
  trojan: TrojanFields,
  hysteria: HysteriaFields,
  anytls: AnyTlsFields,
  mieru: MieruFields,
  tuic: TuicFields,
}

const NO_ERRORS: Record<string, string> = {}

export function ProtocolForm({ type, value, onChange, errors = NO_ERRORS, onEdit }: ProtocolFormProps) {
  const t = useT(['admin-nodes', 'common'])
  const ctx = useMemo<ProtocolContextValue>(
    () => ({
      type,
      value,
      errors,
      t,
      set: (path, v) => {
        onChange(updateSetting(type, value, path, v))
        onEdit?.(path)
      },
    }),
    [type, value, errors, t, onChange, onEdit],
  )
  const Fields = FIELDS[type]
  const notes = compatNotes(type, value)
  return (
    <ProtocolContext value={ctx}>
      <div className="flex flex-col gap-8">
        {notes.length ? (
          <Alert status="warning">
            <Alert.Indicator />
            <Alert.Content>
              <Alert.Title>{t('nodes.ps.compatTitle')}</Alert.Title>
              <Alert.Description>
                <ul className="list-disc ps-4">
                  {notes.map((note) => (
                    <li key={note}>{t(`nodes.ps.compat.${note}`)}</li>
                  ))}
                </ul>
              </Alert.Description>
            </Alert.Content>
          </Alert>
        ) : null}
        {errors[''] ? (
          <p role="alert" className="text-subhead text-label-red">
            {errors['']}
          </p>
        ) : null}
        <Fields />
      </div>
    </ProtocolContext>
  )
}
