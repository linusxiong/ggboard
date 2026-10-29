# Admin guide

**English** · [简体中文](ADMIN_GUIDE.zh-CN.md)

For operators: from the first login to adding nodes, putting plans on sale, setting up payments and routing rules, migrating from
Xboard, the sign-in security policy, Telegram notifications and subscription leak protection.
Installation and the reverse proxy are in [DEPLOY.md](DEPLOY.md); API details are in [API.md](API.md).

---

## 1. First login

```bash
ggboard admin create --email admin@example.com              # without --password, a password is generated and printed once
ggboard admin create --email support@example.com --staff    # support (staff) account
ggboard admin reset-password --email admin@example.com      # reset the password (also ends all of the user's sessions)
ggboard admin promote --email user@example.com              # make an existing user an admin
# Docker (prebuilt image): docker compose -f docker-compose.prebuilt.yml exec ggboard ggboard admin create --email admin@example.com
# Docker (built from source): docker compose -f deploy/docker-compose.yml exec ggboard ggboard admin create --email admin@example.com

```

Passwords need at least 8 characters. Database tables are migrated automatically when commands such as `serve` /
`admin` start, so you do not need to run `ggboard migrate` first.

1. Open `https://<panel domain>/login`, sign in, then go to `/admin` (or choose **Admin** in the account menu).
   Admins land on `/admin/dashboard`, staff on `/admin/tickets`.
2. Do these right away:
   - **Settings → Site**: fill in `app_url` (such as `https://panel.example.com`, without a trailing `/`). Subscription
     links, payment callbacks, email links and passkeys all depend on it.
   - **Profile → Security** (`/profile?tab=security`): turn on two-factor authentication or add a passkey for yourself
     (see §8).
   - **Settings → Email**: configure SMTP and click **Send test email**.
   - **Settings → Node connection**: check `server_token` (a random 32-character value generated on first start).

| Role | Access |
| --- | --- |
| Admin `is_admin` | Every admin page |
| Staff `is_staff` | Tickets, notices, knowledge base; read-only view of users |

---

## 2. Settings tour

The admin **Settings** page (`/admin/settings`) has 11 groups, in this order: **Site, Sign-up & sign-in, Subscription,
Orders, Invites & commission, Node connection, Email, Appearance, Client apps, Security, Telegram**. Each group is
divided into sections. Fields that depend on a switch or a drop-down (allowed email domains, sign-ups per IP, CAPTCHA
keys, SMTP host, distribution rates and so on) only appear once the switch is on. Each group is saved on its own with
**Save** (⌘/Ctrl+S); unsaved edits are kept when you switch groups.
Key names and defaults are defined in `internal/settings/defs.go`. The tables below list every visible key in UI order,
with the UI label in parentheses.

### Site `site`

| Key | Default | Description |
| --- | --- | --- |
| `app_name` | `GG Bond` | Page titles, emails, subscription file names |
| `app_description` / `logo` / `tos_url` | empty | Landing-page description, logo, terms-of-service link on the sign-up page |
| `app_url` | empty | **Required**. The site URL, without a trailing `/` |
| `currency` / `currency_symbol` | `CNY` / `¥` | ISO 4217 code and symbol; all amounts are stored in cents |
| `default_locale` | `zh-CN` | `zh-CN` or `en` |
| `show_info_to_user` | `true` | When off, the user node list does not show hosts and ports |

### Sign-up and sign-in `register`

The page has four sections: **Sign-up**, **Trial**, **Abuse protection** and **CAPTCHA**:

| Key (UI label) | Default | Description |
| --- | --- | --- |
| `stop_register` (Allow sign-ups) | `false` | The UI shows an "Allow sign-ups" switch that is the inverse of the key: turning the switch off means `stop_register: true`. New users cannot register; existing accounts are not affected |
| `email_verify` (Email verification) | `false` | Sign-up requires an email code. Codes are sent by email: first configure SMTP in the **Email** group and send a test email; while the mail driver is still "Log only", a notice here says that codes will not actually be sent. When off, sign-up does not prove ownership of the address (anyone can register with someone else's email). The first password reset of such an account is treated as a change of ownership: the UUID and subscription token are replaced, and passkeys, two-factor authentication and the Telegram binding are cleared |
| `email_whitelist_enable` / `email_whitelist_suffix` (Email domain allow-list / Allowed email domains) | `false` / `[]` | When on, a domain input appears: enter domains only (such as `gmail.com`, exact match), press Return or comma to add one, or paste several at once. The allowed domains are shown under the email field on the sign-up page |
| `invite_force` (Require an invite code) | `false` | Without a valid invite code, sign-up is not possible |
| `try_out_plan_id` / `try_out_hour` (Trial plan / Trial length) | 0 / 0 | Trial plan and length (hours) given at sign-up; the length is hidden when "No trial" is selected |
| `register_limit_by_ip_enable` / `_count` / `_expire` (Limit sign-ups per IP) | `false` / 3 / 60 | The maximum number of accounts one IP can register within N minutes |
| `password_limit_enable` / `_count` / `_expire` (Lock after failed logins) | `true` / 5 / 60 | Allowed failures and lock duration in minutes (counted per email) |
| `captcha_provider` (CAPTCHA) | `none` | `none` / `turnstile` / `recaptcha` / `hcaptcha`; used for sign-up, sign-in and sending email codes |
| `captcha_site_key` / `captcha_secret` (Site key / Secret key) | empty | Appear once a provider is selected: the public site key and the server-side key that is only stored on the server |

### Subscription `subscribe`

The page has six sections: **Subscription hosts**, **Plan changes**, **Content & reminders**, **Routing rules**,
**Leak protection** and **IP locations**:

| Key (UI label) | Default | Description |
| --- | --- | --- |
| `subscribe_url` (Subscription URLs) | `[]` | List of subscription domains; empty uses `app_url`. **One is picked at random every time a link is generated** (to spread the load) |
| `subscribe_update_interval` (Update interval) | 24 | Suggested client update interval (hours, 1–720, sent as `Profile-Update-Interval`) |
| `plan_change_enable` (Allow plan changes) | `true` | When off, users can only renew their current plan |
| `surplus_enable` (Upgrade credit) | `true` | When changing plans, the remaining value of the old plan is credited |
| `reset_traffic_method` (Default traffic reset) | 0 | 0 on the 1st of every month, 1 monthly on the purchase day, 2 never, 3 every year on January 1, 4 yearly on the purchase day; each plan can set its own |
| `show_info_to_server_enable` (Show plan info in subscriptions) | `true` | Inserts remaining traffic / expiry at the top of the subscription node list |
| `show_protocol_to_server_enable` (Show protocol in node names) | `false` | Appends the protocol type to node names |
| `default_remind_expire` / `default_remind_traffic` | `true` | Expiry / traffic reminders that are on by default for new users |
| `allow_user_rule_profile` (Allow custom routing) | `true` | When off, every user gets the default template; subscription profiles, node operations and rule parameters in the link are ignored |
| `ruleset_default_template` (Default routing template) | `standard` | `none` / `minimal` / `standard` / `full` |
| `ruleset_default_source` (Default rule-set source) | `blackmatrix7` | The preferred rule repository |
| `ruleset_mirror` (Rule-set mirror) | empty | GitHub raw proxy prefix or mirror id; empty = direct |
| `subscribe_age_optional` (Allow opting out of encryption) | `true` | After a user turns on age encryption, adding `&age=0` to the link still returns plaintext |
| `subscribe_age_strict` (Strict encryption) | `false` | After a user turns on encryption, formats that do not support age return 406 instead of falling back to plaintext |
| `subscribe_leak_max_ips` (Leak detection threshold) | 0 | Triggers when one subscription token is fetched from more than N distinct IPs (IPv6 per /64) within 24 hours; 0 = off |
| `subscribe_leak_action` (When triggered) | `notify` | Appears when the threshold is above 0: `notify` or `rotate` (rotate the token); see §10 |
| `geoip_enable` (Show IP locations) | `true` | Shows the region and network operator of each IP in the access log (the CZ88 / 纯真 IP database, IPv4 only); updates are checked automatically every day at 05:23 |
| `geoip_download_url` (Database download URL) | latest release of metowolf/qqwry.dat | Switch to a mirror when GitHub is unreachable; empty = no automatic download, only a file you put at `<data_dir>/geoip/qqwry.dat` by hand is used |
| `geoip_version_url` (Version file URL) | `version.json` of metowolf/qqwry.dat | Nothing is downloaded while the version is unchanged; empty = every check downloads the whole file and compares checksums |

The card under the **IP locations** section shows the database version, number of IP ranges, size, download and check
times, and the reason for the last failure. **Update now** runs a check right away (a download that takes longer than 60
seconds moves to the background, and the card refreshes by itself). The button is disabled while the settings have
unsaved changes. Deployment requirements are in DEPLOY.md §6.

### Orders `order`, invites and commission `invite`

| Key | Default | Description |
| --- | --- | --- |
| `order_timeout_minutes` | 30 | Unpaid orders are cancelled automatically (1–10080 minutes) |
| `invite_commission` | 10 | Commission rate % |
| `invite_gen_limit` | 5 | Maximum unused invite codes per user |
| `invite_never_expire` | `true` | When off, an invite code stops working after one use |
| `commission_first_time_enable` | `true` | Commission on the first order only |
| `commission_auto_check_enable` | `true` | A scheduled job pays out commission automatically |
| `commission_withdraw_limit` | 100 | Minimum withdrawal (in site currency units) |
| `commission_withdraw_method` | `["USDT","Alipay"]` | Allowed withdrawal methods (checked by the backend) |
| `withdraw_close_enable` | `false` | Turns withdrawals off; commission can only be moved to the balance |
| `commission_distribution_enable` / `_l1` / `_l2` / `_l3` | `false` / 0 | Three-level distribution rates % |

### Node connection `server`

| Key | Default | Description |
| --- | --- | --- |
| `server_token` | generated automatically | The shared XrayR `ApiKey` / V2bX `ApiKey` / soga `webapi_key` of every node without its own **Node token** (§3). After you click **Regenerate token** and save, the old value stops working immediately |
| `server_pull_interval` / `server_push_interval` | 60 / 60 | Pull / push intervals sent to backends (seconds) |
| `server_white_list` | `[]` | White list sent to soga |
| `device_limit_mode` | 0 | How devices are counted: 0 strict (by online IP), 1 relaxed (an IPv4 /24 or IPv6 /64 counts as one device) |

The **Node backend snippets** section at the bottom of this page generates XrayR / V2bX / soga snippets with the
current `app_url` and token.

### Email `email`

| Key | Default | Description |
| --- | --- | --- |
| `email_driver` | `log` | `log` only writes to the log and sends nothing; `smtp` really sends |
| `email_host` / `email_port` / `email_encryption` | empty / 465 / `ssl` | `ssl`, `tls` (STARTTLS) or `none` |
| `email_username` / `email_password` | empty | SMTP account |
| `email_from_address` / `email_from_name` | empty | Sender address and name |
| `remind_mail_enable` | `true` | Send expiry and traffic reminders |

After saving, click **Send test email** (`POST /api/v1/admin/settings/mail/test`, which uses the saved configuration).

The old `email_template` setting is no longer shown: it is only kept for compatibility with Xboard imports, and the server
never reads it. Customise email content on the **Email templates** page below instead.

### Email templates `/admin/mail-templates`

Open it from the sidebar under **System → Email templates**, or with the **Email templates** button in the top-right
corner of **Settings → Email**. Admins only (staff get 403).
All seven system emails — **Verification code** `verify_code`, **Expiry reminder** `remind_expire`, **Data usage alert**
`remind_traffic`, **Payment received** `order_paid`, **Ticket reply** `ticket_reply`, **Subscription reset**
`subscription_reset` and **General notice** `notify` (bulk emails, subscription leak alerts and so on) — can be
rewritten separately for **简体中文 / English**: the **Subject** and the Markdown **Body**. The body is placed inside the
site's standard email layout (logo or site name, white card, footer), so you only write what goes inside the card. A
locale you have not customised keeps using the built-in template; the list on the left marks each template with
"Chinese / English: Custom / Default".

**Variables**: written the same way as `{{siteName}}` in notices and the knowledge base. Click a variable button in the
editor toolbar to insert it at the cursor; the meaning of each variable is listed under the buttons.
The syntax is `{{name}}`, case-sensitive and without spaces (`{{ code }}` and `{{Code}}` both count as unknown variables
and appear literally in the email). Variables work in the subject too.
Variables marked ★ (a red `*` in the UI) must appear in the **body** (the subject does not count). Every template can use
`{{siteName}}` (site name), `{{siteUrl}}` (`app_url`, may be empty) and `{{year}}` (current year):

| Template | Variables (★ required) | Notes |
| --- | --- | --- |
| Verification code `verify_code` | `code` ★, `minutes` | `minutes` defaults to 10, the same as the built-in email |
| Expiry reminder `remind_expire` | `expiredAt` ★, `url` | `url` is the renewal page (`siteUrl` when not provided) |
| Data usage alert `remind_traffic` | `percent` ★, `used`, `total`, `url` | `percent` is a bare number (such as `86`) |
| Payment received `order_paid` | `tradeNo` ★, `amount` ★, `planName`, `period`, `url` | `amount` includes the currency symbol; `url` is the order page |
| Ticket reply `ticket_reply` | `ticketSubject` ★, `message`, `url` | `message` is the support reply (multi-line) |
| Subscription reset `subscription_reset` | `url` | Sent to the user when an admin uses **Reset subscription secret**; `url` is the user's subscription page (`app_url` + `/subscription`). There is no variable for the new link, so do not put a new link in the email |
| General notice `notify` | `content` ★, `title`, `url` | `title` is the title given by the caller, and the default subject is just `{{title}}`, so bulk emails and leak alerts keep their own subjects; `content` may be multi-line |

Every `url` falls back to `siteUrl` when the caller does not provide one.

**Body**: Markdown (GFM: tables, strikethrough and task lists work, bare URLs become links, a single line break is kept as
a line break, and raw HTML is allowed).
A link that stands alone as a paragraph (for example `[View order]({{url}})`) is rendered as a button in the email; a
link whose address is empty after substitution (no `app_url` configured) is removed.
Quote variables used in raw HTML attributes (`<a href="{{url}}">`). Variable values are escaped and are never treated as
Markdown or HTML.

**Live preview**: while you edit (after a pause of about 0.4 seconds), the real email is rendered with sample data below
the editor; you can switch between **Email / Plain text**. Links in the preview cannot be clicked. Missing required
variables (yellow) and unknown variables (red) are shown as tags right above the preview, so you can fix them before
saving.

**Save, test and restore**:

- **Save** (the save bar at the bottom, or ⌘/Ctrl+S): the server checks again (subject 1–255 characters on one line;
  body at most 64 KB; no unknown variables; all required variables present), and errors are marked on the matching
  field. A saved template takes effect immediately, without a restart.
- **Send test email**: renders **what you are editing** (unsaved changes included) with sample data and sends it to the
  address you enter (your own by default), using the **saved** mail settings; if sending fails, the SMTP error is shown
  under the address field.
- **Restore default**: after you confirm, deletes the customisation for that locale and goes back to the built-in
  template (only available when the template is customised).
- With unsaved changes, switching template or locale, or leaving the page, asks you first.

### Appearance `frontend`, client apps `app`

`frontend_accent`, `frontend_background_url`, `frontend_customer_service_url`, `frontend_footer_html` and
`windows|macos|android|ios_download_url` / `_version` can be saved in the admin console. The download URLs appear in
`downloads` of the public config `GET /api/v1/guest/config`; **the current frontend does not read these appearance and
download settings yet**, so saving them changes nothing on the pages.

### Security `security`

| Key (UI label) | Default | Description |
| --- | --- | --- |
| `passkey_rp_id` (Passkey RP ID) | empty | WebAuthn RP ID (a domain, no port); empty = the host name of `app_url`; passkeys cannot be used on an IP address |
| `passkey_origins` (Extra passkey origins) | `[]` | Extra allowed origins (the origin of `app_url` is always allowed) |
| `admin_mfa_required` (Require 2FA for administrators) | `false` | Forces admins / staff to turn on two-factor authentication or add a passkey (§8) |
| `mfa_issuer` (Authenticator issuer) | empty | The issuer shown in authenticator apps; empty = `app_name` |

### Telegram `telegram`

The page has three sections, **Bot**, **Admin notifications** and **Users**. **Connection** at the bottom has two
buttons, **Send test message** and **Set webhook** (§9).

| Key (UI label) | Default | Description |
| --- | --- | --- |
| `telegram_bot_token` (Bot token) | empty | The bot token from BotFather; when empty, Telegram notifications are off altogether |
| `telegram_webhook_secret` (Webhook secret) | generated on first start | The last segment of the webhook URL (at least 16 characters). After you click **Regenerate secret** next to the field and save (or submit `null` through the API, so the server generates a new 32-character value), you have to **Set webhook** again |
| `telegram_admin_chat_ids` (Admin chat IDs) | `[]` | Chats that receive admin notifications: the numeric ID of a person or group (groups and channels are `-100…`) or `@channel`; press Return after each one to add it |
| `telegram_events` (Events) | Order paid, Admin sign-in, New ticket, Withdrawal request, Subscription leak | Events pushed to the admin chats; see §9.4 |
| `telegram_user_notify` (User notifications) | `true` | Bound users receive their own payment confirmations, ticket replies, and expiry and traffic reminders |
| `telegram_discuss_link` (Community link) | empty | Shown on the user's Telegram card and in the bot's help |

---

## 3. Adding nodes

Order: **groups → routes (optional) → nodes → configure the backend on the server**.

1. **Groups** (`/admin/groups`): a user who buys a plan gets the plan's group and can only use the nodes of that group.
2. **Routes** (`/admin/routes`): routes sent to XrayR / V2bX, with the action `block` (block; also used as soga audit
   rules) or `dns` (resolve matching domains with the given DNS). Block rules take one entry per line, and all lines
   joined with `|` must form a valid Go regular expression.
3. **Nodes** (`/admin/nodes` → **New node**): the protocol type cannot be changed after creation.

| Field | Description |
| --- | --- |
| Address / Client port | Sent to clients; for Hysteria port hopping, enter a range such as `20000-30000` |
| Server port | The port the backend actually listens on |
| Traffic rate | User traffic × rate is billed |
| Groups | With none selected, no user can use the node |
| Parent node | A child node serves as a relay entrance: it reuses the parent's protocol settings, credentials and node token, and its traffic counts on the parent |
| Node token | Empty: the backend uses the shared `server_token`. Set one (the button generates 32 random characters) and the node accepts only it, while the token works for no other node: whoever runs this node can no longer read other nodes' users or report traffic for them. Several nodes may share one token (one soga serving `node_id=1,2,3`). Hidden for child nodes. A copied node gets a new token |
| Show | When off, the node does not appear in user subscriptions, but the backend can still pull its config |
| Protocol / JSON | Protocol parameters (`protocol_settings`); the fields are in [NODE_PROTOCOLS.md](NODE_PROTOCOLS.md) |

Once saved, the node has an ID, and the **Backends** tab of the edit page generates backend snippets for that node
(`<node_token>` stands for the node token when the node, or its parent, has one). Node list statuses: **Online**
(traffic reported), **Idle** (heartbeat only), **Offline**.

When nodes are run by different people, give each node (or each backend instance) its own node token. With one shared
`server_token`, anyone holding it can change `node_id` and read every node's users and keys. Whatever the token,
traffic and online reports only count for users of the node's own groups.

### 3.1 Protocol to backend mapping

| Panel protocol | XrayR `NodeType` | V2bX `NodeType` | soga `server_type` |
| --- | --- | --- | --- |
| vmess | `V2ray` | `vmess` | `v2ray` |
| vless | `Vless` (plus `EnableVless: true`) | `vless` | `vless` |
| trojan | `Trojan` | `trojan` | `trojan` |
| shadowsocks | `Shadowsocks` | `shadowsocks` | `ss` |
| shadowsocksr | — | — | `ssr` |
| hysteria (v1) | — | `hysteria` | — |
| hysteria (v2) | — | `hysteria2` | `hysteria` |
| tuic | — | `tuic` | — |
| anytls | — | `anytls` | `anytls` |
| mieru | — | — | `mieru` |

### 3.2 XrayR (`PanelType: NewV2board`)

```yaml
# /etc/XrayR/config.yml
Nodes:
  - PanelType: "NewV2board"
    ApiConfig:
      ApiHost: "https://panel.example.com"
      ApiKey: "<server_token>"
      NodeID: 1
      NodeType: Vless        # V2ray | Vless | Trojan | Shadowsocks
      EnableVless: true      # vless nodes only
      Timeout: 30
```

XrayR requests `/api/v1/server/UniProxy/{config,user,push}`. It ignores the per-user device limit sent by the panel and
only uses its local `DeviceLimit`, and it has no connection limit. It does not read the pull interval sent by the panel,
so set `ControllerConfig.UpdatePeriodic` to no more than the panel's push interval.

### 3.3 V2bX

```jsonc
// /etc/V2bX/config.json → "Nodes": [ … ]
{
  "Core": "sing",            // sing for hysteria / tuic / anytls, xray for everything else
  "ApiHost": "https://panel.example.com",
  "ApiKey": "<server_token>",
  "NodeID": 1,
  "NodeType": "vless",
  "Timeout": 30
}
```

V2bX uses the pull / push intervals sent by the panel and enforces device limits through
`/api/v1/server/UniProxy/alivelist`. It does not support connection limits (the local `ConnLimit` setting no longer has
any effect). For nodes behind a relay, see §3.5.

### 3.4 soga (`type=soga-v1`)

```ini
# /etc/soga/soga.conf
type=soga-v1
server_type=v2ray
node_id=1
soga_key=<soga license>
api=webapi
webapi_url=https://panel.example.com/soga/
webapi_key=<server_token>
```

We recommend the `/soga/` prefix for `webapi_url` (the soga license is bound to the full `webapi_url`, and the prefix
also makes it easy to rate-limit soga separately on the reverse proxy). soga requests
`/soga/api/v1/{node,users,audit_rules,white_list,traffic,alive_ip,audit_log,status}`; the unprefixed `/api/v1/...`
works as well. soga enforces the speed limit, device limit and **connection limit** sent by the panel (the user's
`conn_limit` is sent as `tcp_limit`), and uses the smaller of these and the local `user_speed_limit` and
`user_conn_limit`.

### 3.5 Nodes behind a relay: device and connection limits

When a relay sits in front of a node, every connection the backend sees comes from the relay's IP. Each user counts as
one device no matter how many devices they use, so **the device limit stops working**, and the IPs under the node's
"online users" are all the relay's IP. There are two ways out:

- **Let the backend see the real IP**: the relay sends PROXY protocol and the V2bX node turns on `EnableProxyProtocol`;
  the device limit then works again.
- **Use a connection limit instead**: fill in **Connection limit** on the plan or the user (`conn_limit`, the number of
  simultaneous TCP connections per user; empty or 0 = unlimited). Only soga supports it (`tcp_limit`); neither XrayR nor
  V2bX can limit connections.

| Limit | XrayR | V2bX | soga |
| --- | --- | --- | --- |
| Speed limit `speed_limit` | Supported | Supported | Supported |
| Device limit `device_limit` | Not supported (local `DeviceLimit` only) | Supported (`alivelist`) | Supported |
| Connection limit `conn_limit` | Not supported | Not supported | Supported (`tcp_limit`) |

---

## 4. Plans, coupons and payments

### 4.1 Plans `/admin/plans`

- **Prices**: entered per period (in the site currency); empty means the period is not offered. Periods: `monthly`,
  `quarterly`, `half_yearly`, `yearly`, `two_yearly`, `three_yearly`, `onetime`, and `reset_traffic` (traffic reset
  pack).
- **Server group**, traffic per period, speed limit, device limit, connection limit (only soga enforces it, see §3.5),
  max users (empty = unlimited), traffic reset method (empty = follow the system setting).
  On purchase (renewals included), on trials, and when users are created or generated in bulk in the admin console, the
  plan's speed limit, device limit and connection limit are copied to the user. Editing a plan does not change existing
  users right away (they get the new values with their next paid order), unless you use `force_update` below.
- **Show** (visible in the shop) / **Sell** (new purchases allowed) / **Renew** (existing users may renew): a plan with
  "Sell" off is only available to existing users for renewal.
- **Apply to current users**: when you edit a plan that users hold, turn on the switch at the end of "Traffic & limits"
  and save (the confirmation dialog states how many users are affected). The group, traffic quota and limits (speed
  limit, device limit, connection limit) are then written to every user holding the plan, expired ones included. Values
  adjusted for individual users are lost; used traffic and expiry dates stay as they are.
  This is `"force_update": true` on `PATCH /api/v1/admin/plans/:id`.

### 4.2 Coupons `/admin/coupons`

The type is **Fixed amount** (entered in whole units of the site currency, such as yuan; stored internally in cents) or
**Percent off** (1–100). You can limit the total number of uses, uses per user, the plans and periods it applies to, and
the validity period. Up to 500 can be generated at once; an empty code is generated at random.

### 4.3 Payment methods `/admin/payments`

**Add payment method** → choose a gateway → fill in the gateway settings → set the display name, icon and handling fee
(fixed + percentage, paid by the user) → enable it.
The gateway cannot be changed after creation.

**Icon**: click a built-in icon in the icon grid; the form shows a live preview of what users see at checkout. Built-in
icons are saved as `preset:<id>`:

| Icon | `<id>` | Icon | `<id>` |
| --- | --- | --- | --- |
| Alipay | `alipay` | Bank transfer | `bank` |
| WeChat Pay | `wechat` | QR code | `qrcode` |
| Stripe | `stripe` | Wallet | `wallet` |
| Card | `card` | EPay | `epay` |
| PayPal | `paypal` | Epusdt | `epusdt` |
| Crypto | `crypto` | BTCPay | `btcpay` |
| Bitcoin | `btc` | Coinbase | `coinbase` |
| USDT | `usdt` | Ethereum | `eth` |

When you create a payment method, an icon is picked automatically from the gateway and its settings; once you pick one by
hand, it no longer changes automatically:

- EPay: `type` `alipay` → Alipay, `wxpay` / `wechat` → WeChat Pay, `usdt…` → USDT, anything else or empty → EPay;
- Stripe: `payment_methods` `card` → Card, `alipay` → Alipay, `wechat_pay` → WeChat Pay, `auto` → Stripe;
- Epusdt: `currency_type` empty or `usdt…` → USDT, anything else (such as `tron.trx`) → Crypto;
- Alipay F2F → Alipay, Coinbase → Coinbase, BTCPay → BTCPay.

Choose **Custom URL** to enter an image URL (`https://…` or an on-site `/…` path; a square SVG/PNG works best) or a
single emoji. The full format is in [API.md › Payment icons](API.md#payment-icons). Legacy icon values imported from
Xboard do not get in the way of editing other fields.

The notify URL is shown after saving, in this form:

```text
<notify_domain, or app_url>/api/v1/guest/payment/notify/<gateway>/<uuid>
```

`uuid` is unique per payment method. After paying, the buyer returns to the order page through
`/api/v1/guest/payment/return/<trade_no>`.

| Gateway `gateway` | Required settings | Callback setup |
| --- | --- | --- |
| EPay `epay` | `url` (without `/submit.php`), `pid`, `key`; optional `type` (`alipay`/`wxpay`/`qqpay` and so on) | Sent automatically with each order, nothing to configure; CNY only |
| Epusdt / BEpusdt `epusdt` | `api_url`, `api_token`; `currency_type` defaults to `usdt.trc20` | Sent automatically |
| Alipay F2F `alipay_f2f` | `app_id`, `private_key` (RSA2 ≥ 2048), `alipay_public_key` (the Alipay public key); `sandbox` | Sent automatically; public-key mode only, CNY only |
| Stripe `stripe` | `secret_key`, `webhook_secret`; optional `currency`, `exchange_rate`, `payment_methods` (`auto`/`card`/`alipay`/`wechat_pay`) | In Dashboard → Developers → Webhooks, add an endpoint with the notify URL and subscribe it at least to `checkout.session.completed` and `checkout.session.async_payment_succeeded` (`async_payment_failed` and `expired` are optional and only acknowledged) |
| Coinbase Business `coinbase` | `api_key_name`, `api_key_secret` (ECDSA P-256 PEM), `webhook_secret`; `exchange_rate` (USDC per unit of site currency) | Create a subscription for `checkout.payment.*` (and optionally `checkout.refund.*`) with `cdp data webhooks subscriptions create`, targeting the notify URL; only `checkout.payment.success` activates an order |
| BTCPay Server `btcpay` | `server_url`, `store_id`, `api_key` (`btcpay.store.cancreateinvoice` + `btcpay.store.canviewinvoices`), `webhook_secret` | Create a webhook in Store → Settings → Webhooks with the notify URL as the Payload URL; an order is activated only when the invoice is `Settled` and carries the binding signature written by the panel (unpaid invoices created before the webhook secret was rotated need manual confirmation) |

The notify URLs of Stripe / Coinbase / BTCPay must be public HTTPS. Protocol details for each gateway are in
[PAYMENTS_GATEWAYS.md](PAYMENTS_GATEWAYS.md).

### 4.4 Orders `/admin/orders`

- **Filters**: the segmented control at the top switches by status (All / Pending / Processing / Cancelled / Completed /
  Credited); you can also filter by order number, user (email or ID), commission status, plan, payment method and date.
  The filters are kept in the address bar.
- **Mark as paid** (activates the plan right away and calculates commission; verify the payment with the payment
  provider first) and **Cancel order** (returns the balance used and the coupon use) are only available for pending
  orders.
- **Commission status** (order details): Pending → **Paying out** means approved; `commission_settle` pays it to the
  inviter every hour and changes it to **Valid**. **Invalid** means it is not paid. Only the scheduled job can set
  "Valid" (choosing it by hand is refused), and commission that has been paid can no longer be changed.
- **Assign order**: creates a pending order for a user (email or ID); an empty amount uses the plan price. The user can
  pay it themselves, or you can mark it as paid.

---

## 5. Rule sets and templates

- **Settings → Subscription**: `ruleset_default_template`, `ruleset_default_source`, `ruleset_mirror`,
  `allow_user_rule_profile` (see §2).
- **Rule sets** (`/admin/rulesets`): the tabs **Sources / Categories / Templates / Mirrors** show the effective result of
  the built-in catalog merged with your overrides (read-only). Built-in sources: blackmatrix7, MetaCubeX, Loyalsoldier,
  SagerNet, ACL4SSR, DustinWin. Built-in templates: `none` (global proxy), `minimal` (China direct), `standard`
  (standard routing), `full` (fine-grained routing). Built-in mirrors: `direct`, `ghfast`, `gh-proxy`, `ghproxy-net`,
  `gh-llkk`.
- **Custom overrides** (the fifth tab): the JSON has the same structure as the built-in catalog. `sources` merge by key,
  and `categories` / `templates` / `mirrors` by `id`: an existing id is replaced, a new id is appended. Once saved
  (`PUT /api/v1/admin/rulesets`), the overrides apply right away to newly generated subscriptions; submit `null` to go
  back to the built-in catalog.
  The form can quickly add a mirror or a custom category. A new category's id is stored in users' routing profiles, so do
  not change it later.

The catalog and template formats are in [RULESETS.md](RULESETS.md), the subscription output in
[SUBSCRIPTION.md](SUBSCRIPTION.md).

---

## 6. Users and content

- **Users**: search by email; the segmented control filters by status (Any status / Active / Banned), and there are also
  plan, group, role and expiry filters and **More filters** (User ID, Inviter ID, Subscription token, Remarks). Toolbar:
  **New user**, **Generate** (downloads a CSV that includes the passwords), **Send mail** (to the users matching the
  current filters; the body is sent as plain text with blank lines separating paragraphs, and a background queue sends it
  asynchronously), **Export CSV**.
- **User details** has three tabs, **Summary / Profile / Subscription**. Summary shows the plan, traffic, subscription
  token and short link, and the bound Telegram chat ID; Profile edits every field; the **Subscription** tab shows the
  age encryption status and the access log (see §10). The more menu of each row: **Log in as user** (a one-time link,
  valid for 5 minutes, best opened in a private window; cannot be used for admin or staff accounts), **Reset
  subscription secret** (the UUID and subscription token are regenerated, the user has to re-import, and their sign-in
  sessions end; the system emails the user, and sends a Telegram message if they are bound, telling them to sign in again
  to get the new link — the notice does not contain the new link), **Reset traffic**, **Ban / Unban**, and Delete (which
  also removes their orders, tickets, traffic records, subscription profiles and keys).
- **Tickets / Notices / Knowledge**: staff can use these too. The segmented control at the top of the ticket list
  switches between **Awaiting / Open / Closed / All**; you can search by user email or ID, and ↑/↓ moves through the
  list. Replying to a closed ticket reopens it. Notices and knowledge articles use a Markdown editor (Write / Preview).
  Knowledge article bodies support the placeholders `{{siteName}}`, `{{subscribeUrl}}`, `{{urlEncodeSubscribeUrl}}` and
  `{{safeBase64SubscribeUrl}}`. Content between `<!--access start-->…<!--access end-->`, which **Subscribers only** in
  the toolbar inserts, is only shown to users with an active plan. The last three subscription placeholders expand to
  each reader's own subscription URL (including the token), so **only admins can write them**: when staff save a new
  body containing them, it is refused (403), but staff can still change the title, category and visibility of an
  admin's article.
- **System** (`/admin/system`): runtime status, scheduled jobs, mail queue, cache and logs, refreshed every 30 seconds.

---

## 7. Migrating from Xboard

```bash
ggboard import xboard --dsn "mysql://user:pass@127.0.0.1:3306/xboard" --dry-run   # rehearse first (everything is rolled back)
ggboard import xboard --dsn "mysql://user:pass@127.0.0.1:3306/xboard" --with-stats
```

| Flag | Description |
| --- | --- |
| `--dsn` | Required: `mysql://…`, `postgres://…` or `sqlite:///path` |
| `--driver` | `mysql` / `postgres` / `sqlite`, when the DSN has no scheme |
| `--dry-run` | Runs every read and write, then rolls back |
| `--force` | Deletes existing ggboard data (users, plans, nodes…) before importing |
| `--skip-banned` | Does not import banned users or the data that belongs only to them (tickets, invite codes, stats…); references to them in rows that are kept are set to null, and the summary shows `skipped banned N`. Note that the ban itself is not carried over, so these emails can register again in ggboard; pass this flag on the dry run, the real import and any `--force` re-import. The rules are in IMPORT_XBOARD.md §3.5 |
| `--with-stats` / `--stats-days` | Also import stats, the last 90 days by default |
| `--source-tz` | Xboard time zone, default `Asia/Shanghai` |

**Before importing**: pause sales and wait until pending orders are finished (orders are not imported); settle
commission in Xboard; run `--dry-run` first and read every warning.

**Checklist after importing**:

1. Restart ggboard so the running process loads the imported settings.
2. Backends: a `server_token` of at least 16 characters is copied as is, so the XrayR / V2bX / soga keys stay the same.
   A shorter one (or an empty one) is not imported and ggboard keeps the token it generated: copy it from
   **Settings → Node connection** into every backend, as the import report says. Point `ApiHost` /
   `webapi_url` at ggboard (or move the old domain's DNS), and change any `NodeID` that was renumbered, as the warnings
   say.
3. Payments: EPay / Epusdt / Alipay F2F need no change; for Stripe, Coinbase and BTCPay, change the webhook to the new
   notify URL in the provider's dashboard before enabling them.
4. Subscriptions: `curl -sI "https://<host>/api/v1/client/subscribe?token=<real token>"` should return 200 with
   `subscription-userinfo`; the `/s/<token>` short link works too. Make sure `subscribe_url` includes the domains users
   already use.
5. Sign-in: sign in once each as a user with a bcrypt hash and a user with an old md5/sha256 hash (after a successful
   sign-in the hash is upgraded to argon2id automatically).
6. Settings: send a test email and check the CAPTCHA keys again. Telegram settings are not imported: fill them in again
   and set the webhook as in §9.5. Xboard's `secure_path` no longer has any effect; to hide `/admin`, restrict it on the
   reverse proxy.
7. Keep the Xboard database as a read-only backup (the importer never modifies it).

The full mappings and the meaning of each warning are in [IMPORT_XBOARD.md](IMPORT_XBOARD.md).

---

## 8. 2FA and passkey policy

- **Require MFA for admins**: with `admin_mfa_required` on, admins and staff who have neither two-factor authentication
  nor a passkey get `403 mfa_setup_required` from the admin API; the frontend tells them and sends them to
  **Profile → Security** to set it up (that page is not restricted). Turn it on for yourself before you turn on the
  switch.
  Regular users are not affected by this switch and can turn it on if they want.
- **Passkey prerequisites**: `app_url` (or `passkey_rp_id`) must be set; otherwise `passkey_enabled` is `false` in the
  public config and the frontend hides passkeys. Users must visit the site through its official domain (HTTPS).
- **Several domains**: set `passkey_rp_id` to the common parent domain (such as `example.com`) and add the other origins
  (such as `https://www.example.com`) to `passkey_origins`. A passkey is bound to the RP ID it was registered with, so
  existing passkeys stop working after you change `passkey_rp_id`.
- **TOTP secret encryption**: authenticator secrets are stored encrypted with a key derived from `jwt.secret`. Rotating
  `jwt.secret` invalidates every bound authenticator (and also ends every session).
- **Recovery**: a user who loses their authenticator can sign in with a backup code, or directly with a passkey they
  added. The admin console and the CLI have **no** way to reset another user's two-factor authentication, and
  `ggboard admin reset-password` does not turn it off either. As a last resort, an operator deletes the user's row in
  the `user_mfa` table (which turns off TOTP and backup codes), or the matching rows in the `passkeys` table.

---

## 9. Telegram notifications

The panel pushes key events to admins through a Telegram bot, and lets users bind their own Telegram to receive order and
ticket notifications.

### 9.1 Create the bot

1. In Telegram, open [@BotFather](https://t.me/BotFather), send `/newbot` and follow the prompts to enter a name and a
   username (it must end in `bot`).
2. The `123456789:AA…` that BotFather replies with is the token. Enter it in **Settings → Telegram → Bot token**
   (`telegram_bot_token`) and save.
   The token is the bot's password: do not share screenshots of it. If it leaks, `/revoke` it in BotFather to get a new
   one.
3. Optional: `/setdescription` and `/setuserpic` set the description and avatar; for `/setcommands`, enter
   `start - Link your panel account`, `unbind - Unlink`, `id - Show the chat ID`.

### 9.2 Set the webhook

The bot receives commands through a webhook. First make sure **Site → `app_url`** is a publicly reachable HTTPS address
(Telegram only accepts ports 443, 80, 88 and 8443), then click **Set webhook** under **Settings → Telegram → Connection**
(`POST /api/v1/admin/settings/telegram/webhook`). The panel registers
`<app_url>/api/v1/guest/telegram/webhook/<telegram_webhook_secret>` with Telegram (message updates only, with a
`secret_token`). On success it shows "Webhook set: <url>"; on failure it shows the error code and description returned
by Telegram. When the bot token or `app_url` is missing, that field is marked red. Both buttons are disabled while the
Telegram settings have unsaved changes, so save first.
Set the webhook again after changing `app_url`, regenerating the webhook secret or changing the token. A token can only
have one webhook at a time: if an old panel (such as Xboard) still uses this bot, it stops receiving commands once you
set the webhook here.

### 9.3 Get chat IDs and test

1. **Person**: send `/id` to the bot in a private chat; it replies `Chat ID: 123456789` (`/id` works in any chat, even
   before binding).
2. **Group**: add the bot to the group and send `/id` in the group (`/id@your_bot` when there are several bots) to get
   an ID starting with `-100…`.
3. **Channel**: make the bot an admin of the channel and enter `@channel_username` directly in **Admin chat IDs**.
4. Enter these IDs one by one in **Admin chat IDs** (`telegram_admin_chat_ids`, press Return after each) and save, then
   click **Send test message** (`POST …/telegram/test`, sent synchronously). The page shows "Sent N, failed M" and lists
   each failed chat with the reason (common failures: `chat not found` = wrong ID, or the bot is not in the group;
   `bot was blocked` = the bot was blocked).

### 9.4 Events

**Events** (`telegram_events`) decides which events the admin chats receive:

| Message title | Key (name in the settings) | Content |
| --- | --- | --- |
| 🧾 New order | `order_new` (New order) | A user placed an order: email, plan, period, amount, order number (orders assigned by hand in the admin console are not pushed) |
| 💰 Order paid | `order_paid` (Order paid) | The above plus payment method and transaction ID; **on by default** |
| 🧾 Order cancelled | `order_cancel` (Order cancelled) | Cancelled automatically on timeout or by an admin (cancellations by the user are not pushed) |
| 🔐 Admin sign-in | `admin_login` (Admin sign-in) | Every admin / staff sign-in: email, role, method (password, password + 2FA code, password + backup code, passkey), IP, device (UA), time; **on by default** |
| 🎫 New ticket | `ticket_new` (New ticket) | Subject, priority, user and a summary of the first message; **on by default** |
| 🎫 New reply to your ticket | `ticket_reply` (Ticket reply) | Every reply from support (these go to the user anyway; admins usually do not need them) |
| 💸 Withdrawal request | `withdraw` (Withdrawal request) | User, amount, method and the masked account; **on by default** |
| 🚨 Possible subscription leak | `subscription_leak` (Subscription leak) | The user caught by leak detection and the number of distinct IPs (§10); **on by default** |

**Sign-in alerts**: keep `admin_login` on, and every sign-in to any admin or staff account (passkey sign-ins included)
sends a message. If you see an IP or device you do not recognise, change that account's password right away (which ends
all of its sessions) and check its two-factor authentication. Sign-ins of regular users are not pushed.

User side (when `telegram_user_notify` is on, bound users only): their own successful order payments, ticket replies,
expiry reminders and traffic reminders, the "🚨 Possible subscription leak" alert about their own link
(`subscription_leak`, sent whether or not the admins subscribe to it above; §10), plus "🔄 Subscription reset" after an
admin uses **Reset subscription secret** (`subscription_reset`, sent only to that user and not one of the events above).
Expiry / traffic reminders are sent with the daily 10:00 reminder job and share the "once per 24 hours" limit with the
email reminders. Turning off `remind_mail_enable` only stops the emails; users bound to Telegram still get the Telegram
reminders.

A failed send is retried 3 times automatically (1 s / 2 s / 4 s; when Telegram rate-limits, the panel waits as long as
Telegram asks). Sends that still fail are recorded in **System → Logs** (WARN level). Queued, sent and failed counts and
the enabled channels are in the `notify` field of `GET /api/v1/admin/system/status` (not shown in the admin console
yet).
If the server cannot reach `api.telegram.org` directly, notifications cannot be delivered; give the server working
outbound network access.

### 9.5 Migrating from Xboard

The importer keeps each user's `telegram_id`. If you keep Xboard's existing bot token, users who were already bound get
notifications without binding again; remember to move the webhook to ggboard as in §9.2. Xboard's `telegram_*`
settings are not imported and have to be filled in here again:

- **Bot token**: Xboard's `telegram_bot_token` (or a new one from BotFather);
- **Community link**: Xboard's `telegram_discuss_link`;
- **Admin chat IDs**: Xboard sends admin notifications to the admin accounts that are bound to Telegram; ggboard sends
  them to the chats listed here instead. An admin account's chat ID is the Telegram row in **User details → Summary**;
  you can also get it by sending `/id` to the bot in a private chat;
- Tick the **Events** you need, save, then click **Set webhook** and **Send test message**.

---

## 10. Subscription protection

Configured under **Settings → Subscription → Leak protection** (§2). The two mechanisms are independent of each other:

**age-encrypted subscriptions**: users generate a key and turn encryption on themselves on the subscription page (see
[USER_GUIDE.md](USER_GUIDE.md) §3.4). Once it is on, only the Clash Meta (mihomo) format is encrypted; other formats
still get plaintext by default.

| Setting | Effect |
| --- | --- |
| **Allow opting out of encryption** `subscribe_age_optional` (on by default) | Returns plaintext when the link has `&age=0`, for quick troubleshooting |
| **Strict encryption** `subscribe_age_strict` (off by default) | Formats that do not support age (Stash, sing-box, Surge and so on) return 406 instead of falling back to plaintext |

The defaults lean towards compatibility: a leaked link can still get plaintext with `&age=0` or by switching to another
format. For real leak protection, turn on **Strict encryption**, turn off **Allow opting out of encryption**, and remind
users who have encryption on to only use clients that support age. The settings only affect users who have encryption
on.

**Leak detection**: active when **Leak detection threshold** `subscribe_leak_max_ips` is above 0. It triggers when one
subscription token is fetched from more than N distinct IPs (IPv6 counted per /64) within 24 hours, at most once every
24 hours per token:

- **When triggered** = `notify` ("Notify (email the user; Telegram to admins and the bound user)" in the UI): the user
  gets a "Your subscription link may have leaked" email; the subscription is not affected;
- **When triggered** = `rotate` ("Notify and rotate the link" in the UI): the user's subscription token is rotated first
  (the UUID and sign-in sessions stay the same, the old link stops working immediately), then the new link is emailed to
  the user;
- In both cases a WARN log line is written, and when "Subscription leak" is ticked in **Events**, an alert is pushed to
  the admin Telegram chats (§9.4). A user who has bound Telegram also gets the alert there while **User notifications**
  is on, whatever **Events** says; that message does not contain the new link.

Set the threshold to the normal number of IPs of a household or multi-device user (for example 10–20); a threshold that
is too low gives false alarms. Access records are written in batches every 10 seconds; the latest 100 per user are kept,
for at most 30 days. Counting is per token, so the new link after a rotation or reset starts counting from zero.

**Looking at one user**: the **Users → User details → Subscription** tab shows:

- the age encryption status (On / Off) and the public key (admins cannot see the private key). **Reset age key**
  generates a new key pair for the user and leaves the switch as it is; the user has to read the new private key on the
  subscription page and update their clients (`POST /api/v1/admin/users/:id/subscription/age/reset`);
- the number of distinct IPs in the last 24 hours and the latest 100 access records (full IP and location, client,
  format, token tag, time; `GET /api/v1/admin/users/:id/subscription/access-log`). Rows with the same token tag come
  from the same subscription link;
- **Rotate link**: the admin side has no separate token rotation endpoint, so this button calls **Reset subscription
  secret**. The UUID and token are replaced together, the user has to re-import the subscription, their sign-in sessions
  end, and they are notified by email (and Telegram, if bound). If you only want to change the link, ask the user to use
  **Rotate link** on their own subscription page.

These endpoints and the tab are admin-only; staff cannot see them.
