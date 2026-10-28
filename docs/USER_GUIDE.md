# User guide

**English** · [简体中文](USER_GUIDE.zh-CN.md)

For users: signing up and signing in, buying a plan, importing the subscription into each client, subscription
profiles and node operations, encrypted subscriptions and the access log, two-factor authentication and passkeys,
Telegram notifications, tickets, and invites and commission. Menu names are those of the English interface.

---

## 1. Sign up and sign in

- **Create account** (`/register`): email + password (at least 8 characters). When the site requires email
  verification, click **Send code** and enter the email code. The invite code is optional, and required when the site
  enforces invites. An invite link shared by a friend, `/register?code=<invite-code>`, fills in the invite code for you.
  The site may restrict email domains or turn on a human check (Turnstile / reCAPTCHA / hCaptcha).
- **Sign in** (`/login`): email + password. With two-factor authentication on, you then enter the 6-digit code (or a
  backup code), or click **Use a passkey instead**.
- **Sign in with a passkey**: no email needed. Pick a passkey saved on your device or in your password manager.
  Supported browsers also list your passkeys directly in the autofill menu of the email field.
- **Forgot password?** (`/forgot`): set a new password with an email code. If you did not verify your email when you
  signed up, the first password reset counts as confirming that the account is yours: the subscription link and node
  credentials change (every client has to import the subscription again), and passkeys, two-factor authentication and
  the Telegram binding are cleared and need to be set up again.
- Too many wrong passwords in a row lock you out for a while. Wait as the page says, then try again.

---

## 2. Buying a plan

1. **Shop**: choose a plan and a billing period (Monthly, Quarterly, Half-yearly, Yearly, 2 years, 3 years, One-time).
   When your plan has expired or its traffic is used up you can buy a **Traffic reset**: it clears the traffic used in
   the current cycle right away and leaves the expiry date as it is.
2. **Confirm order**: you can enter a coupon code. Your account balance is applied automatically. When you change plans,
   the remaining value of the current plan is credited (the amount on the order page is what counts).
   You can have only one unpaid order at a time: go to that order, or cancel it and continue.
3. **Order details**: choose a payment method, then **Pay now**. Depending on the gateway you are sent to a payment
   page, shown a QR code, or shown an embedded payment page. After you pay, the page updates the status by itself.
   When your balance and discounts cover the full amount, click **Activate**.
4. Order status: Unpaid → Activating → Completed. An order that is not paid in time is cancelled automatically, and any
   balance it used is returned on cancellation. You can filter by status at the top of **Orders**.

---

## 3. Importing the subscription

Open **Subscription** (`/subscription`). From top to bottom the page has: the subscription profile tabs (see 4.1), the
current profile's **Subscription link** (copy, QR code, one-click import), the editor tabs **Routing rules / Node
operations / Config preview**, and two cards, **Subscription security** (access log, rotation and reset) and
**Subscription encryption**. The subscription URL looks like this:

```text
https://<subscription-domain>/api/v1/client/subscribe?token=<your token>
```

Do not share this link. The site picks the output format from the client's User-Agent, and the
`Subscription-Userinfo` response header carries used traffic, total traffic and the expiry time, which most clients
show directly.

### 3.1 One-click import

| Client | Platforms | Link it opens |
| --- | --- | --- |
| Clash / mihomo | Clash Verge · FlClash · CMFA | `clash://install-config?url=<subscription-url>` |
| sing-box | SFA · SFI · SFM · Hiddify | `sing-box://import-remote-profile?url=<subscription-url>` |
| Shadowrocket | iOS | `shadowrocket://add/sub://<base64url(subscription-url)>?remark=<site-name>` |
| Stash | iOS · macOS | `stash://install-config?url=<subscription-url>` |
| Surge | iOS · macOS | `surge:///install-config?url=<subscription-url>` |
| Quantumult X | iOS | `quantumult-x:///add-resource?remote-resource=…` (added as a `server_remote`) |
| Loon | iOS | `loon://import?sub=<subscription-url>` |

On a computer, click **Show QR code** and scan it with a mobile client to import. For v2rayN, v2rayNG, NekoBox, Karing,
Passwall and similar clients, **Copy** the subscription URL and add it in the client by hand.

### 3.2 Formats and query parameters

If automatic detection fails, append `&flag=<format>` to the link: `clash-meta`, `clash`, `stash`, `singbox`, `surge`,
`surfboard`, `quantumultx`, `shadowrocket`, `loon`, `v2ray` (Base64 share links), `ssr`. Other optional parameters:

| Parameter | Effect |
| --- | --- |
| `profile=<link-id>` | Use one of your subscription profiles (see 4.1; once you switch to that profile, the subscription link already carries it). A template name, `none` / `minimal` / `standard` / `full`, makes the default profile use that template for this request without changing your saved rules. Any other value (including the ID of a deleted profile) uses the default profile |
| `source=` · `cats=a,b` · `final=` · `mirror=` | Temporarily override the rule source, the enabled categories, the final policy and the rule download mirror (`0` = direct) |
| `types=vless,trojan` | Only output nodes of these protocols |
| `filter=香港,HK` | Only output nodes whose name or tags contain any of the keywords (case-insensitive) |
| `emoji=0` | Remove emoji from node names |
| `list=1` | Surge / Surfboard / Loon / Quantumult X: output only the node lines |
| `age=0` | Get plain text for this request while encrypted subscriptions are on (only when the site allows it, see 3.4) |

When the site turns off "custom routing", the rule parameters are ignored. On the default profile, choosing a "Template
for this link" option in the **Routing rules** dropdown next to the subscription link adds `profile=<template-name>` to
the link automatically.

### 3.3 Rotate or reset

Both buttons are on the **Subscription security** card of the subscription page (**Reset subscription** is also under
**Profile → Subscription**):

- **Rotate link**: changes only the token in the link, and the old link stops working immediately. The node password
  (UUID), active connections and sign-ins stay as they are; just put the new link on each device. Use this first when
  you suspect the link has leaked. The links of all your subscription profiles change together.
- **Reset subscription**: changes both the token and the UUID (node password). The old link and the old node
  configuration all stop working, every device has to import again, active connections drop, and your other devices are
  signed out (the current page stays signed in).
- When the site has leak detection on, you get an email (and a Telegram message if you have bound Telegram) if one
  subscription link is fetched from too many different IPs (IPv6 counted per /64) within 24 hours. The site may also be
  set to rotate the link automatically; the email then includes the new link (the Telegram message does not).
- An administrator can also reset your subscription. You get an email titled "Your <site name> subscription was reset by
  the administrator" (and a Telegram message if you have bound Telegram). The old link no longer works and you have been
  signed out; sign in again, copy the new link on the subscription page and import it again. The notice does not contain
  the new link; it only points to this site's subscription page.

### 3.4 Encrypted subscriptions (age)

When this is on, the subscription content is encrypted with your [age](https://age-encryption.org/) public key, and only
a client that holds the private key can decrypt it: even if someone gets hold of your subscription link, all they can
download is ciphertext. For now only the mihomo (Clash Meta) format is encrypted, and it needs **mihomo 1.19.27 or
later** as the core.

1. On the subscription page, **Subscription encryption** card → **Generate key**, then click **Show private key** /
   **Copy private key** to get `AGE-SECRET-KEY-1…` (you can view it again at any time). A newly generated key does not
   turn encryption on; set up your clients first.
2. Enter the private key in your client (**How to set up your client** on the card says the same):
   - **Clash Party (formerly Mihomo Party, 1.9.4 or later)**, **FlClash**: edit this subscription, paste the key into the
     Age private key field, save, then update the subscription.
   - **mihomo core (1.19.27 or later) / your own config**: reference the subscription URL with `proxy-providers` and add
     `age-secret-key`:

     ```yaml
     proxy-providers:
       ggboard:
         type: http
         url: "https://<subscription-domain>/api/v1/client/subscribe?token=<your token>&flag=clash-meta"
         interval: 3600
         age-secret-key: AGE-SECRET-KEY-1...
     proxy-groups:
       - {name: Proxy, type: select, use: [ggboard]}
     rules:
       - MATCH,Proxy
     ```

     `proxy-providers` imports only the nodes, so the routing rules have to be written in this local config. When you
     run the core directly on a whole encrypted config, give it the private key with the `-age-secret-key` command-line
     flag or the `CLASH_AGE_SECRET_KEY` environment variable (the private key cannot go in the config file).
   - **Clash Verge Rev** has no private key field: create a local profile and reference the subscription with the
     `proxy-providers` snippet above (the bundled mihomo core decrypts it).
   - If your client has no age option, update it to the latest version first.
3. Once your clients update without problems, go back to the subscription page and turn on **Encrypt subscription
   content**. From then on every Clash Meta (mihomo) format subscription is encrypted (it starts with
   `-----BEGIN AGE ENCRYPTED FILE-----`), and traffic and expiry information still shows as usual.
4. Stash, sing-box, Surge, Quantumult X, Loon, Shadowrocket, v2rayN and other clients cannot decrypt. By default the
   site still gives them plain text; when the site turns on "strict mode", these clients are refused (406) and cannot
   update the subscription. If you need plain text for a while, append `&age=0` to the Clash Meta link (when the site
   allows it, the card says so).
5. After **Regenerate**, the old private key stops working immediately, and every client needs the new private key (the
   switch stays as it is). **Delete key** goes back to plain text. The private key is as good as the subscription itself:
   paste it only into your own clients. If the site changes its internal encryption key, the private key can no longer
   be shown. The public key keeps working; regenerate when you need the private key.
6. The **Config preview** on the subscription page always shows plain text.

### 3.5 Access log

**Recent access** on the **Subscription security** card of the subscription page lists the last 50 subscription
updates: time, client, format and IP (the last part of an IPv4 address is hidden; for IPv6 only the first 64 bits are
shown). It also shows how many different IPs fetched the subscription in the last 24 hours. The links of all your
subscription profiles share this log. A location or client you do not recognise means the link may have leaked; click
**Rotate link**.

---

## 4. Routing rules

Routing rules live in the **Routing rules** tab of the editor on the subscription page (read-only when the site
administrator has turned customisation off). Every subscription profile has its own:

1. **Template**: None (global proxy), Minimal (China direct), Standard, Full. Switching templates overwrites your
   changes to categories, policy groups and the final policy.
2. **Rule source**: blackmatrix7, MetaCubeX, Loyalsoldier, SagerNet, ACL4SSR, DustinWin. When the current source lacks a
   category, or has no rule files for a client (blackmatrix7, for example, has no sing-box rule sets), that category
   switches to another source automatically, so you never switch sources by hand for one client. **Source each client
   gets**, below the source list, shows the source each client ends up with (Clash Meta always prefers MetaCubeX).
3. **Categories**: grouped as "Basics / AI services / Messaging / Streaming / Platforms & services / Games / Regions &
   fallback". Turn on the services you want to route (such as OpenAI, Claude, Telegram, Netflix) and pick a target
   policy for each category: a policy group, Direct (DIRECT) or Block (REJECT).
4. **Policy groups**: the type is Manual select, Auto (URL test), Fallback or Load balance. **Members** is comma
   separated and can hold other group names, `DIRECT`, `REJECT`, `*` (all nodes) or a regex matching node names (such
   as `香港|HK`). `Proxy` is the required main group; it cannot be renamed or deleted.
5. **More settings**: the final policy, the rule download mirror (GitHub (direct) / Site default mirror / a mirror
   site), the DNS mode (Default / Fake-IP / No DNS block), and mainland China IPs going direct (`GEOIP,CN,DIRECT`).

After **Save rules** the subscription link uses them right away; clients pick them up on their next subscription
update. The **Config preview** tab shows what each client format generates from the saved rules. Rule sets are
delivered to mihomo / Clash Meta, Stash, sing-box, Surge, Surfboard, Loon and Quantumult X. Shadowrocket, v2rayN and
similar clients receive only nodes; set their rules in the client.

### 4.1 Multiple profiles

The tabs at the top of the subscription page are your **subscription profiles** (such as "Default", "Phone", "Work"),
up to 10. Each profile has its own subscription link, routing rules and node operations, so you can import different
ones on different devices.

- **New profile**: enter a name and a **Link ID** (lower-case letters, digits, `-` and `_`, at most 32 characters, not
  a template name; leave it empty to generate one), and start from the "Site default template" or a copy of an existing
  profile. The link ID cannot be changed after creation.
- The more menu next to a tab: **Rename** (changes only the name), **Duplicate**, **Make default**, **Delete**. The
  default profile cannot be deleted.
- Once you switch to a profile, the subscription link above is its link (links of profiles other than the default carry
  `&profile=<link-id>`). Links without the parameter (including old links already imported into clients) use the
  **default profile**, and **Make default** moves them to the new default profile. After you delete a profile, links
  with its ID fall back to the default profile too.
- If you have unsaved changes when you switch profiles, you are asked whether to discard them. When the site turns off
  "custom routing" you can only view, and every subscription uses the site's default rules (node operations have no
  effect either).

### 4.2 Node operations

The editor's **Node operations** tab (similar to Sub-Store's node operations) tidies the current profile's node list.
It applies to every client format and leaves the traffic / expiry entries at the top alone:

| Setting | Effect |
| --- | --- |
| Node types | Keep only the selected protocols (vless, hysteria2, ss, …); none selected = all |
| Regions | Keep only the selected regions (HK, TW, JP, SG, US, KR, DE, GB…; `UK` counts as GB); none selected = all. Regions are detected from a flag, a Chinese or English place or city name (香港, Tokyo…) or a region code (`HK01`, `JP-2`) in the node name or node tags; nodes whose region cannot be detected are removed |
| Include (regex) / Exclude (regex) | RE2 regexes that keep or remove nodes by name, e.g. `香港\|HK`, `过期\|剩余\|官网`; start with `(?i)` to ignore case. At most 32 in total for both, each ≤ 256 bytes; lookarounds and backreferences are not supported |
| Rename | Runs "Match → Replace with" pairs in order. Reference groups with `$1` or `${1}` (write `${1}` when a letter or digit follows), `$$` stands for `$`, and an empty replacement deletes the matched text |
| Prefix / Suffix | Add fixed text to every node name (at most 32 characters each) |
| Flag emoji | Keep as is / Add automatically (adds one by region when the name has no flag) / Remove all |
| Sort | Panel order / By name (natural sort, `HK 2` before `HK 10`) / By region (in the order of the selected regions, or a built-in order when none are selected) |
| Remove duplicate nodes | Of nodes with the same protocol, server address and port, keep only the first |
| Maximum nodes | Keep at most the first N nodes (0 = no limit, at most 1000) |

Processing order: filters (types, regions, include, exclude) → dedupe → rename → flags → prefix / suffix → sort →
limit; names that still collide at the end get 2, 3… appended. **Live preview** simulates the result in your browser
with the nodes currently available to you, and shows how many nodes are kept and which were removed and why; the
subscription itself is what counts. When you are done, click **Save node operations**; **Clear node operations**
restores the defaults. The full keyword list for region detection is in [SUBSCRIPTION.md](SUBSCRIPTION.md).

---

## 5. Account security

All of this is under **Profile → Security** (`/profile?tab=security`).

### 5.1 Two-factor authentication (TOTP)

1. Click **Turn on 2FA** and scan the QR code with an authenticator app (1Password, Google Authenticator, Microsoft
   Authenticator and so on), or enter the key by hand. On a phone you can tap **Open in an authenticator on this
   device**.
2. Enter the 6-digit code to finish.
3. **Save your backup codes**: each backup code (format `xxxxx-xxxxx`) works only once and is shown only this one time;
   copy them or download a `.txt`. When you are running low, click **Generate new backup codes** (this asks for a code,
   and all old backup codes stop working).
4. Turning off two-factor authentication needs your current password and a code; the backup codes stop working with it.

If you lose your phone, sign in with a backup code or with a passkey you have added, then set it up again. If you have
neither, only an administrator can help.

### 5.2 Passkeys

- **Add a passkey**: first give it a name (such as MacBook, iPhone, 1Password), then follow the prompts of your browser
  or password manager. Passkeys can be stored in iCloud Keychain, 1Password, Bitwarden and the like, and synced across
  devices. The same device or password manager cannot add a second passkey for the same account.
- You can **Rename** each passkey. **Remove** asks for your current password; remember to remove the passkey from the
  device or password manager too.
- On the sign-in page, **Sign in with a passkey** signs you in without a password. Accounts with two-factor
  authentication on need no code when they sign in with a passkey.
- You have to open the site on its official domain; otherwise you see "This passkey doesn't match this address". When
  the site has not set up passkeys, the sign-in page shows no passkey option.

### 5.3 Change password

After **Change password** every device is signed out, and you sign in again with the new password.

---

## 6. Password managers

Sign-in, sign-up, password reset and password change are all standard forms: the email field has
`autocomplete="username"`, password fields `current-password` / `new-password`, code fields `one-time-code`, and the
email field on the sign-in page also has `webauthn` (passkey autofill). So 1Password, Bitwarden / Vaultwarden, iCloud
Keychain and Chrome all save and fill them correctly:

- When you sign up or reset your password, you can let the password manager generate a strong password. A password
  manager that supports TOTP can fill in the two-factor code.
- A password manager's "change password" shortcut opens `/.well-known/change-password`, which redirects to
  `/profile?tab=security`.
- `/.well-known/passkey-endpoints` tells password managers where to add and manage passkeys (the same page).

---

## 7. Tickets, notices and knowledge

- **Tickets**: **New ticket** asks for a subject, a priority (Low / Medium / High) and a description of the problem.
  You can have only one open ticket at a time: continue in that ticket, or **Close ticket** and then open a new one.
  Status: Awaiting reply / Replied / Closed.
- **Notices** and **Knowledge**: announcements and client guides published by the site. Some articles need an active
  plan to view.

---

## 8. Invite and commission

The **Invite** page:

- **Generate code** (the number of codes is capped, and only unused codes are shown), then copy the invite code or the
  invite link `/register?code=<invite-code>`, or show a QR code.
- When friends sign up through your link and buy a plan, you earn commission at the commission rate: once an order
  completes, it goes into "Pending commission", and after settlement it counts towards "Commission earned".
  **Commission history** lists every order.
- **Move to balance**: move available commission into your account balance to pay for plans.
- **Withdraw**: enter a withdrawal method (such as USDT or Alipay; it must be one the site allows) and the receiving
  account; you need at least the minimum withdrawal amount. Submitting creates a ticket that staff process by hand (so
  close any other open ticket first). The site may turn withdrawals off; then you can only move commission to your
  balance.

---

## 9. Telegram notifications

When the site has the Telegram bot enabled, you can bind your account to your own Telegram and receive successful order
payments, ticket replies, and subscription expiry and traffic reminders there.

1. Open **Profile → Account** and click **Bind Telegram** on the **Telegram** card. The dialog shows an **Open in
   Telegram** button and a QR code (the link is `https://t.me/<bot>?start=<code>`).
2. Open the link on your phone or computer (or scan the QR code with your phone). Telegram opens a chat with the bot;
   tap **Start**. The bot replies "Linked to …", and the panel dialog shows "Telegram bound" by itself. You can also
   send `/start <code>` to the bot by hand. A code is valid for 10 minutes and works once; **Get a new code** makes the
   old code invalid.
3. The card shows the bound Telegram username. When the site has a community group, it also shows **Join the discussion
   group**.

- **Unbind**: click **Unbind** on the Telegram card, or send `/unbind` to the bot in Telegram.
- **Switch accounts**: bind again with another Telegram account. A Telegram account can be bound to only one panel
  account; binding it to a new account unbinds the old one automatically.
- The notifications you can get: successful payments of your own orders, ticket replies, plan expiry reminders, traffic
  reminders, suspected subscription leak alerts, and the notice after an administrator resets your subscription (none
  are sent when the site turns off "User notifications").
- Expiry and traffic reminders follow the "Remind me before my plan expires" and "Remind me when my traffic is running
  out" switches under **Profile → Account → Email reminders**. They go out by email and Telegram together, at most once
  per type within 24 hours.
- When the site has not enabled the bot, the card shows "The administrator has not enabled Telegram yet."
- Bot commands: `/start <code>` binds, `/unbind` unbinds, `/id` shows the Chat ID.
