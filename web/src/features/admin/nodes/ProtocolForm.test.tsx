import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Suspense, useState } from 'react'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import { loadNamespaces } from '@/i18n'
import type { NodeType } from '@/lib/api/types'
import { useUiStore } from '@/stores/ui'
import { defaultSettings, finalizeSettings, type Settings, updateSetting } from './protocol'
import { ProtocolForm } from './ProtocolForm'

function Harness({
  type,
  initial,
  onChange,
  errors,
}: {
  type: NodeType
  initial: Settings
  onChange: (s: Settings) => void
  errors?: Record<string, string>
}) {
  const [value, setValue] = useState(initial)
  return (
    <ProtocolForm
      type={type}
      value={value}
      errors={errors}
      onChange={(next) => {
        setValue(next)
        onChange(next)
      }}
    />
  )
}

const start = (type: NodeType, edits: Array<[string, unknown]> = []) =>
  edits.reduce((ps, [p, v]) => updateSetting(type, ps, p, v), defaultSettings(type))

beforeAll(async () => {
  useUiStore.setState({ locale: 'zh-CN' })
  await loadNamespaces(['admin-nodes', 'common'])
})

describe('<ProtocolForm> per node type', () => {
  it.each<[NodeType, Array<[string, unknown]>, RegExp, string, string, unknown]>([
    [
      'vmess',
      [['network', 'ws']],
      /^Host 请求头/,
      'cdn.example.com',
      'network_settings.headers.Host',
      'cdn.example.com',
    ],
    ['vless', [['tls', 2]], /^私钥/, 'PRIVATE_KEY', 'tls_settings.private_key', 'PRIVATE_KEY'],
    [
      'shadowsocks',
      [['cipher', '2022-blake3-aes-128-gcm']],
      /^服务端密钥/,
      'MDEyMzQ1Njc4OWFiY2RlZg==',
      'server_key',
      'MDEyMzQ1Njc4OWFiY2RlZg==',
    ],
    ['shadowsocksr', [], /^节点密码/, 'node-pass', 'password', 'node-pass'],
    [
      'trojan',
      [['network', 'grpc']],
      /^gRPC 服务名/,
      'trojan-grpc',
      'network_settings.serviceName',
      'trojan-grpc',
    ],
    ['hysteria', [['obfs', 'salamander']], /^混淆密码/, 'obfs-pass', 'obfs_password', 'obfs-pass'],
    [
      'anytls',
      [['tls', 2]],
      /^REALITY server_names/,
      'a.example.org, b.example.org',
      'tls_settings.server_names',
      ['a.example.org', 'b.example.org'],
    ],
    ['tuic', [], /^SNI/, 'tu.example.com', 'server_name', 'tu.example.com'],
  ])(
    '%s: typing into the form writes protocol_settings',
    async (type, edits, label, text, path, expected) => {
      const onChange = vi.fn<(s: Settings) => void>()
      render(
        <Suspense fallback={null}>
          <Harness type={type} initial={start(type, edits)} onChange={onChange} />
        </Suspense>,
      )
      const input = await screen.findByLabelText(label)
      await userEvent.setup().type(input, text)
      const last = onChange.mock.lastCall?.[0]
      expect(last).toBeDefined()
      const final = finalizeSettings(type, last!)
      const actual = path
        .split('.')
        .reduce<unknown>((o, k) => (o as Record<string, unknown> | undefined)?.[k], final)
      expect(actual).toEqual(expected)
    },
  )

  it('mieru renders its selects with the current values', async () => {
    render(
      <Suspense fallback={null}>
        <Harness type="mieru" initial={defaultSettings('mieru')} onChange={vi.fn()} />
      </Suspense>,
    )
    expect(await screen.findByText('传输')).toBeInTheDocument()
    expect(screen.getAllByText('低').length).toBeGreaterThan(0)
    // Compatibility: one warning line on the page, the backend details behind its ⓘ.
    expect(screen.getByText('后端兼容性提示')).toBeInTheDocument()
    expect(screen.queryByText(/仅 soga 支持/)).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: '关于「后端兼容性提示」的说明' }))
    expect(await screen.findByRole('dialog')).toHaveTextContent(/仅 soga 支持/)
  })

  it('shows server errors on the matching input', async () => {
    render(
      <Suspense fallback={null}>
        <Harness
          type="vless"
          initial={start('vless', [['tls', 2]])}
          onChange={vi.fn()}
          errors={{ 'tls_settings.private_key': 'Reality 需要私钥' }}
        />
      </Suspense>,
    )
    const input = await screen.findByLabelText(/^私钥/)
    expect(input).toHaveAttribute('aria-invalid', 'true')
    expect(screen.getByText('Reality 需要私钥')).toBeInTheDocument()
  })
})
