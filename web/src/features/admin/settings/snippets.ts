/** Node backend config snippets (docs/NODE_PROTOCOLS.md §1.1) filled with `app_url` + `server_token`. */
export function buildSnippets(appUrl: string, token: string) {
  const host = (appUrl || 'https://panel.example.com').replace(/\/+$/, '')
  const key = token || '<server_token>'
  return {
    xrayr: [
      '# /etc/XrayR/config.yml → Nodes[]  (NodeType: V2ray | Vmess | Vless | Trojan | Shadowsocks)',
      'PanelType: "NewV2board"',
      'ApiConfig:',
      `  ApiHost: "${host}"`,
      `  ApiKey: "${key}"`,
      '  NodeID: 1',
      '  NodeType: Vless',
      '  EnableVless: true',
    ].join('\n'),
    v2bx: [
      '// /etc/V2bX/config.json → Nodes[]  (NodeType: vmess | vless | trojan | shadowsocks | hysteria2 | tuic | anytls)',
      '{',
      '  "Core": "sing",',
      `  "ApiHost": "${host}",`,
      `  "ApiKey": "${key}",`,
      '  "NodeID": 1,',
      '  "NodeType": "vless",',
      '  "Timeout": 30',
      '}',
    ].join('\n'),
    soga: [
      '# /etc/soga/soga.conf  (server_type: v2ray | vless | trojan | ss | ssr | hysteria | anytls | mieru)',
      'type=soga-v1',
      'server_type=v2ray',
      'node_id=1',
      'soga_key=<soga license>',
      'api=webapi',
      `webapi_url=${host}/soga/`,
      `webapi_key=${key}`,
    ].join('\n'),
  }
}
