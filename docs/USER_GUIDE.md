# 用户指南 / User guide

面向用户：注册登录、购买套餐、在各客户端导入订阅、定制分流规则、两步验证与通行密钥、工单和邀请返利。
菜单名称以中文界面为准，括号内为英文界面名称。

*For end users: sign up, buy a plan, import the subscription into your client, build routing rules, secure the
account with 2FA and passkeys, open tickets and invite friends.*

---

## 1. 注册与登录 / Sign up and sign in

- **注册**（`/register`）：邮箱 + 密码（至少 8 位）。站点开启邮箱验证时点 **发送验证码** 并填写邮箱验证码；
  邀请码可选，站点强制邀请时必填。好友分享的邀请链接 `/register?code=<邀请码>` 会自动填好邀请码。
  站点可能限制邮箱后缀，或启用人机验证（Turnstile / reCAPTCHA / hCaptcha）。
- **登录**（`/login`）：邮箱 + 密码；开启了两步验证时再输入 6 位验证码（或备用码），也可以点 **改用通行密钥验证**。
- **使用通行密钥登录**：无需输入邮箱，选择设备或密码管理器里保存的通行密钥即可；支持的浏览器还会在邮箱输入框的自动填充菜单里直接列出通行密钥。
- **忘记密码**（`/forgot`）：通过邮箱验证码设置新密码。
- 连续输错密码会被暂时锁定，按页面提示等待后再试。

---

## 2. 购买套餐 / Buying a plan

1. **购买套餐（Shop）**：选择套餐与付费周期（月付、季付、半年付、年付、两年付、三年付、一次性）。已到期或流量用尽时可购买
   **流量重置包**，立即清零本周期已用流量，到期时间不变。
2. **确认订单**：可输入优惠码；账户余额会自动抵扣；更换套餐时原套餐剩余价值会折抵（以订单页金额为准）。
   同一时间只能有一个待支付订单，可前往该订单或取消后继续。
3. **订单详情**：选择支付方式后 **去支付**。根据网关不同会跳转支付页、显示二维码或嵌入支付页面；支付后页面自动刷新状态。
   余额与优惠覆盖全部金额时点 **确认开通** 即可。
4. 订单状态：待支付 → 开通中 → 已完成；超时未支付会自动取消，取消时已抵扣的余额自动退回。

---

## 3. 导入订阅 / Importing the subscription

打开 **订阅（Subscription）**（`/subscription`）。订阅地址形如：

```text
https://<订阅域名>/api/v1/client/subscribe?token=<你的 token>
```

请勿泄露该链接。站点通过客户端的 User-Agent 自动输出对应格式，并在响应头 `Subscription-Userinfo`
中带上已用流量、总流量与到期时间，大多数客户端会直接显示。

### 3.1 一键导入 / One-click import

| 客户端 | 适用平台 | 打开的链接 |
|---|---|---|
| Clash / mihomo | Clash Verge · FlClash · CMFA | `clash://install-config?url=<订阅地址>` |
| sing-box | SFA · SFI · SFM · Hiddify | `sing-box://import-remote-profile?url=<订阅地址>` |
| Shadowrocket | iOS | `shadowrocket://add/sub://<base64url(订阅地址)>?remark=<站点名>` |
| Stash | iOS · macOS | `stash://install-config?url=<订阅地址>` |
| Surge | iOS · macOS | `surge:///install-config?url=<订阅地址>` |
| Quantumult X | iOS | `quantumult-x:///add-resource?remote-resource=…`（作为 `server_remote` 添加） |
| Loon | iOS | `loon://import?sub=<订阅地址>` |

在电脑上可点 **显示二维码**，用手机客户端扫码导入。v2rayN、v2rayNG、NekoBox、Karing、Passwall 等客户端请
**复制** 订阅地址后在客户端里手动添加。

### 3.2 指定格式与高级参数 / Formats and query parameters

自动识别失败时，在链接末尾追加 `&flag=<格式>`：`clash-meta`、`clash`、`stash`、`singbox`、`surge`、`surfboard`、
`quantumultx`、`shadowrocket`、`loon`、`v2ray`（Base64 分享链接）、`ssr`。其他可选参数：

| 参数 | 作用 |
|---|---|
| `profile=<规则方案>` | 使用你保存的某个规则方案（见 4.1，订阅页会直接给出每个方案的链接）；填模板名 `none` / `minimal` / `standard` / `full` 时临时使用该模板，不改动已保存的规则；其他值使用默认方案 |
| `source=` · `cats=a,b` · `final=` · `mirror=` | 临时覆盖规则来源、启用的分类、兜底策略、规则下载镜像（`0` 直连） |
| `types=vless,trojan` | 只输出这些协议的节点 |
| `filter=香港,HK` | 只输出名称或标签包含任一关键字的节点（不区分大小写） |
| `emoji=0` | 去掉节点名中的 emoji |
| `list=1` | Surge / Surfboard / Loon / Quantumult X 只输出节点行 |
| `age=0` | 开启了加密订阅时临时获取明文（站点允许时才有效，见 3.4） |

站点关闭“自定义分流”时，规则相关参数会被忽略。订阅页的 **分流规则** 下拉框选择“临时使用模板”时，会自动给链接加上
`profile=`。

### 3.3 更换订阅链接 / Rotate or reset

- **更换订阅链接**：只换订阅 token，旧链接立即失效；节点密码（UUID）与登录状态不变，在各设备上更新为新链接即可。
  怀疑链接泄露时优先使用。
- **重置订阅**：同时更换订阅地址和 UUID（节点密码），旧链接和旧节点配置全部失效，所有设备都需要重新导入，并会退出其他设备的登录。
- 站点开启了泄露检测时，同一个订阅链接 24 小时内被过多不同 IP（IPv6 按 /64 计）获取会给你发邮件提醒；站点也可能设置为
  自动更换订阅链接，此时邮件里会附上新链接。

### 3.4 加密订阅（age）/ Encrypted subscriptions

开启后，订阅内容用你的 [age](https://age-encryption.org/) 公钥加密，只有持有私钥的客户端能解开：即使订阅链接被别人拿到，
也只能下载到一段密文。目前只有 mihomo（Clash Meta）格式会加密，需要 **mihomo 1.19.27 或更新** 的内核。

1. 订阅页 **加密订阅** → **生成密钥**，复制私钥 `AGE-SECRET-KEY-1…`（之后随时可以再次查看）。
2. 在客户端里填写私钥：
   - **Clash Party（原 Mihomo Party）**：编辑订阅信息 → **Age 解密密钥**。
   - **FlClash**：添加或编辑配置时填写 **age 私钥**。
   - **mihomo 内核 / 自己写配置**：用 `proxy-providers` 引用订阅地址并加上私钥：

     ```yaml
     proxy-providers:
       ggboard:
         type: http
         url: "https://<订阅域名>/api/v1/client/subscribe?token=<你的 token>&flag=clash-meta"
         interval: 3600
         age-secret-key: AGE-SECRET-KEY-1...
     proxy-groups:
       - {name: Proxy, type: select, use: [ggboard]}
     rules:
       - MATCH,Proxy
     ```

     直接运行内核加载整份加密配置时，用命令行参数 `-age-secret-key` 或环境变量 `CLASH_AGE_SECRET_KEY` 提供私钥。
   - **Clash Verge Rev** 目前不能解密订阅：新建一个本地配置，按上面的 `proxy-providers` 写法引用订阅（由内置的 mihomo 内核解密）。
3. 客户端能正常更新后，回到订阅页打开 **启用加密**。之后 mihomo 格式的订阅都是加密内容（以 `-----BEGIN AGE ENCRYPTED FILE-----` 开头），
   流量与到期信息照常显示。
4. 其他客户端（Stash、sing-box、Surge、Shadowrocket、v2rayN 等）不支持解密：站点默认仍给它们明文；站点开启“严格模式”时会拒绝这些客户端。
   需要临时明文时可在链接后加 `&age=0`（站点允许时）。
5. **重新生成密钥** 后旧私钥立即失效，需要在所有客户端里换成新私钥；**删除密钥** 即关闭加密。私钥只保存在你的账户里，请不要分享。

### 3.5 订阅访问记录 / Access log

订阅页的 **访问记录** 列出最近 50 次订阅更新：时间、客户端、格式和 IP（隐藏最后一段，IPv6 只显示前 64 位）。
出现陌生的地区或客户端时，说明链接可能已泄露，请更换订阅链接。

---

## 4. 分流规则 / Routing rules

订阅页下方的 **分流规则** 编辑器（站点管理员关闭自定义时为只读）：

1. **模板**：无规则（全局代理）、精简（国内直连）、标准分流、精细分流。切换模板会覆盖分类、策略组与兜底策略的修改。
2. **规则来源**：blackmatrix7、MetaCubeX、Loyalsoldier、SagerNet、ACL4SSR、DustinWin；当前来源缺少某个分类时自动使用其他来源。
3. **分类**：按“基础 / AI 服务 / 通讯 / 流媒体 / 平台与服务 / 游戏 / 地区与兜底”分组，开启需要分流的服务
   （如 OpenAI、Claude、Telegram、Netflix）并为每个分类选择目标策略：某个策略组、直连（DIRECT）或拦截（REJECT）。
4. **策略组**：类型可选手动选择、自动测速、故障转移、负载均衡；**包含** 用逗号分隔，可写其他策略组名、`DIRECT`、
   `REJECT`、`*`（全部节点）或匹配节点名的正则（如 `香港|HK`）。`Proxy` 是必需的主策略组，不能重命名或删除。
5. **其他设置**：兜底策略、规则下载镜像（直连 GitHub / 站点默认镜像 / 镜像站）、DNS 模式（默认 / Fake-IP / 不下发 DNS）、
   中国大陆 IP 直连（`GEOIP,CN,DIRECT`）。

点 **保存规则** 后订阅链接自动生效，客户端下次更新订阅即可。**配置预览** 可查看各客户端格式的生成结果。

### 4.1 多个规则方案 / Multiple profiles

最多可保存 10 个规则方案（如“默认”“公司电脑”“手机”），每个方案有自己的规则与节点处理设置。

- **新建方案** 时可从已有方案复制；方案的英文标识（slug，如 `work`）会出现在订阅链接里，可改名或修改标识。
- 订阅页为每个方案给出单独的订阅链接（在链接后加 `&profile=<标识>`）；不带参数的链接使用 **默认方案**，可随时切换默认方案。
- 默认方案不能删除；站点关闭“自定义分流”时所有方案只读，订阅统一使用站点默认规则。

### 4.2 节点处理 / Node operations

每个方案还可以整理节点列表（类似 Sub-Store 的节点操作），对所有客户端格式生效，不影响开头的流量/到期信息条目：

| 设置 | 作用 |
|---|---|
| 地区 | 只保留选中地区（HK、TW、JP、SG、US、KR、DE、UK…）。地区按节点名里的国旗、中英文地名或城市名（香港、Tokyo…）、地区代码（`HK01`、`JP-2`）识别 |
| 包含 / 排除 | 正则表达式（RE2），按节点名保留或去掉，例如 `香港\|HK`、`(?i)premium`；`(?i)` 表示不区分大小写 |
| 协议 | 只保留 vless、hysteria2（`hy2`）、ss 等协议 |
| 重命名 | 按顺序执行“匹配 → 替换”，替换里可用 `$1`、`${1}` 引用分组，替换为空即删除匹配的文字 |
| 国旗 | 保留 / 补全（名字里没有国旗时按地区加上）/ 去掉 |
| 前缀 / 后缀 | 给每个节点名加上固定文字 |
| 排序 | 默认（站点顺序）/ 按名称（自然排序，`HK 2` 在 `HK 10` 前）/ 按地区（按所选地区的顺序） |
| 去重 | 协议、服务器地址和端口都相同的节点只保留第一个 |
| 数量上限 | 最多保留前 N 个节点 |

执行顺序：筛选 → 去重 → 重命名 → 国旗 → 前缀后缀 → 排序 → 数量上限。地区识别规则的完整关键词表见 `docs/SUBSCRIPTION.md`。
规则集会下发给 mihomo / Clash Meta、Stash、sing-box、Surge、Surfboard、Loon、Quantumult X；Shadowrocket 与
v2rayN 等只接收节点，规则需在客户端内设置。

---

## 5. 账户安全 / Account security

都在 **个人中心 → 安全**（`/profile?tab=security`）。

### 5.1 两步验证 / Two-factor authentication (TOTP)

1. 点 **开启两步验证**，用身份验证器 App（1Password、Google Authenticator、Microsoft Authenticator 等）扫描二维码，
   或手动输入密钥；在手机上可直接点 **在本设备的验证器中打开**。
2. 输入 6 位验证码完成绑定。
3. **保存备用码**：每个备用码（格式 `xxxxx-xxxxx`）只能用一次，只显示这一次，可复制或下载 `.txt`。
   剩余不多时点 **重新生成备用码**（需输入验证码，旧备用码全部失效）。
4. 关闭两步验证需要当前密码和验证码，备用码随之失效。

丢失手机时：用备用码登录，或用已添加的通行密钥登录，再重新绑定。两者都没有时只能联系管理员。

### 5.2 通行密钥 / Passkeys

- **添加通行密钥**：先起个名字（如 MacBook、iPhone、1Password），再按浏览器或密码管理器提示创建。通行密钥可保存在
  iCloud 钥匙串、1Password、Bitwarden 等并跨设备同步。同一设备或密码管理器不能为同一账户重复添加。
- 每个通行密钥可 **重命名**；**删除** 需要输入当前密码，并记得同时在设备或密码管理器中移除它。
- 登录页点 **使用通行密钥登录** 即可免密码登录；开启了两步验证的账户用通行密钥登录时不再需要验证码。
- 必须通过站点正式域名访问，否则会提示“通行密钥与当前网址不匹配”。站点未配置通行密钥时，登录页不显示通行密钥入口。

### 5.3 修改密码 / Change password

**修改密码** 后所有设备都会退出登录，需要用新密码重新登录。

---

## 6. 密码管理器 / Password managers

登录、注册、找回密码与修改密码都是标准表单：邮箱字段 `autocomplete="username"`，密码字段
`current-password` / `new-password`，验证码字段 `one-time-code`，登录页邮箱字段还带 `webauthn`（通行密钥自动填充）。
因此 1Password、Bitwarden / Vaultwarden、iCloud 钥匙串与 Chrome 都能正确保存和填充：

- 注册与找回密码时可直接让密码管理器生成强密码；两步验证码可由支持 TOTP 的密码管理器自动填写。
- 密码管理器的“修改密码”入口会访问 `/.well-known/change-password`，自动跳转到 `/profile?tab=security`。
- `/.well-known/passkey-endpoints` 告诉密码管理器在哪里添加和管理通行密钥（同一页面）。

---

## 7. 工单、公告与知识库 / Tickets, notices and knowledge

- **工单（Tickets）**：**新建工单** 填写主题、优先级（低 / 中 / 高）与问题描述。同一时间只能有一个处理中的工单，
  请在原工单中继续沟通，或 **关闭工单** 后再新建。状态：等待回复 / 已回复 / 已关闭。
- **公告（Notices）** 与 **使用文档（Knowledge）**：站点发布的通知和客户端教程；部分文档内容需要有效套餐才能查看。

---

## 8. 邀请返利 / Invite and commission

**邀请返利（Invite）** 页面：

- **生成邀请码**（数量有上限，只显示未使用的码），复制邀请码或邀请链接 `/register?code=<邀请码>`，也可显示二维码。
- 好友通过链接注册并购买后，你按佣金比例获得佣金：订单完成后进入“待结算佣金”，结算后计入“累计获得佣金”，
  **佣金记录** 列出每笔订单。
- **转入余额**：把可用佣金转入账户余额，用于购买套餐。
- **申请提现**：填写提现方式（如 USDT、Alipay，须为站点允许的方式）与收款账户；需达到最低提现金额。提交后会创建一个工单，
  由工作人员人工处理（因此需要先关闭其他未关闭的工单）。站点可能关闭提现，此时只能转入余额。
