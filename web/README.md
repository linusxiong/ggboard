# GG Bond — web

React 19 + TypeScript 7 + Vite 8 (Rolldown) + Tailwind CSS v4 + **HeroUI v3** + TanStack (Router, Query, Table, Form,
Virtual) + zustand. Apple HIG design tokens. Everything below is a **convention for feature agents** — read it together
with `AGENTS.md`, `MISTAKES.md` and `docs/FRONTEND.md` (verified library cheat sheet).

```bash
pnpm install
pnpm dev                 # http://localhost:5173, /api and /s/ → http://localhost:8080 (GGBOARD_API=… to override)
pnpm typecheck && pnpm lint && pnpm test && pnpm build && pnpm lh   # quality gates — all must pass
pnpm routes              # regenerate src/routeTree.gen.ts (typecheck/lint/test do it first)
pnpm build               # vite build + scripts/prerender.mjs (public routes; needs Chrome, else skipped)
pnpm preview:mock        # serve dist/ like the Go server, with fixture API (demo login: any email/password)
pnpm lh                  # build, then Lighthouse desktop AND mobile on /, /login, /register, /forgot (asserts 100s)
LH_BASE=http://127.0.0.1:18090 pnpm lh:mobile           # same against a running Go binary
LH_BASE=… LH_TARGET=auth pnpm lh:desktop                 # signed-in /dashboard, /subscription (LH_USER/LH_PASSWORD)
BASE=http://127.0.0.1:18090 pnpm ui:smoke --axe          # crawl every page: console/network errors, i18n leaks, overflow, axe
BASE=http://127.0.0.1:18090 node scripts/screenshots.mjs # README screenshots → ../docs/screenshots
pnpm format              # prettier
```

Dev-only mock backends (in-memory, fake data on example.com, never bundled): `pnpm exec vite --config
scripts/mock/vite.config.ts` (dev server + user-portal mock), or `node scripts/mock/server.ts` (admin console mock on
`:8787`) together with `GGBOARD_API=http://localhost:8787 pnpm dev`; `scripts/mock/{commerce,admin-a}.ts` are smaller
area mocks (see their header comments).

## Layout

```text
src/
  main.tsx                 bootstrap: theme, <html lang>, `core` dictionary, router
  router.tsx               createRouter (preload on intent, default pending/error/404, 401 → /login,
                           403 mfa_setup_required → /profile?tab=security)
  routes/                  file-based routes (TanStack Router plugin → routeTree.gen.ts, generated + git-ignored)
    __root.tsx             tiny root: Suspense + lazy toast region + 404
    index.tsx              landing · -landing/ = lazy below-the-fold sections ("-" = ignored by the generator)
    login.tsx register.tsx forgot.tsx · -login/MfaStep.tsx (TOTP / backup-code step)
    _user.tsx              pathless layout (signed in): /dashboard /shop /orders /orders/$tradeNo /subscription
                           /invite /tickets /tickets/$id /knowledge /notices /profile (tabs, ?tab=security)
    _admin.tsx             pathless layout (is_admin || is_staff) for /admin/* (/admin → dashboard or tickets)
    _admin/admin/*.tsx     dashboard users plans groups nodes routes orders coupons payments tickets notices
                           knowledge settings rulesets system (staff: users, tickets, notices, knowledge)
  features/<area>/         page bodies, dialogs and helpers per area: dashboard, shop, orders, subscription, invite,
                           tickets, content, security, admin/<page>/ (route files stay thin)
  lib/api/                 client.ts, types.ts, guest.ts, passport.ts, user.ts, admin.ts, queries.ts, index.ts
  lib/                     guards, format, toast, motion, theme, errors, authErrors, button, clipboard, cn,
                           publicData, queryClient, validators
  hooks/                   useLiteForm, useFinishAuth, useCooldown, useCssVar
  stores/                  auth.ts (token/user, `gg-auth`), ui.ts (theme/locale/sidebar, `gg-ui`), storage.ts
  i18n/                    index.ts + <locale>/<namespace>.json (zh-CN, en)
  components/              shared UI (see below)
  styles/                  index.css (entry), theme.css (Apple tokens), utilities.css (@utility classes)
  test/                    vitest setup + cross-feature tests (autofill, login/MFA, security, …)
scripts/                   gen-routes.mjs, heroui-css.ts (per-component CSS), preview.ts (prod-like preview + mock
                           API), fixtures/, mock/ (dev mock backends)
public/                    favicon.svg, apple-touch-icon.png, manifest.webmanifest, robots.txt
```

## Adding a page

1. Create the route file, e.g. `src/routes/_user/orders/index.tsx` (URL `/orders`) or
   `src/routes/_admin/admin/users.tsx` (URL `/admin/users`). The dev server/build regenerates `routeTree.gen.ts`.
2. Export **only** `Route` from a route file (anything else breaks code splitting). Put the page body and helpers in
   `src/features/<area>/` (or a `-something/` folder next to the route; the `-` prefix is ignored by the generator).
3. Wrap the body in `<PageShell title=… description=… actions=…>`; it renders the single `<h1>` and sets `document.title`.
4. Admin-only pages: `beforeLoad: requireAdmin`. Staff pages (users read-only, tickets, notices, knowledge): nothing extra.
5. Navigation entries live in `src/components/layout/nav.ts` (labels in `i18n/*/nav.json`; `staff: true` shows an
   admin entry to staff).

```tsx
import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { PageShell } from '@/components/PageShell'
import { loadNamespaces, useT } from '@/i18n'
import { ordersQuery } from '@/lib/api/queries'

export const Route = createFileRoute('/_user/orders/')({
  validateSearch: (s: Record<string, unknown>) => ({ page: Number(s.page ?? 1) || 1 }),
  loaderDeps: ({ search: { page } }) => ({ page }),
  // `context.queryClient` exists in every _user/_admin child (the layouts put it there).
  loader: ({ context, deps }) =>
    Promise.all([
      context.queryClient.query({ ...ordersQuery(deps), staleTime: 'static' }),
      loadNamespaces(['orders']),
    ]),
  component: OrdersPage,
})

function OrdersPage() {
  const t = useT(['orders', 'common'])
  const { page } = Route.useSearch()
  const { data } = useQuery(ordersQuery({ page }))
  return <PageShell title={t('title')}>{/* … */}</PageShell>
}
```

## Data: API + React Query

- **Never call `fetch` directly.** Endpoint functions for every route the SPA uses live in `src/lib/api/{guest,passport,user,admin}.ts`
  (2FA/passkey calls included; `src/features/security/webauthn.ts` wraps `@simplewebauthn/browser`):
  `import { userApi, adminApi } from '@/lib/api'` → `userApi.orders({ page })`, `adminApi.updateUser(id, patch)`.
- The client (`lib/api/client.ts`) unwraps `{data}` → `T` and `{data,total}` → `Page<T> = { items, total }`, adds
  `Authorization: Bearer` and `Accept-Language`, times out after 15 s and throws
  `ApiError { status, code, message, errors, retryAfter, fieldError(f) }` (`retryAfter` = the 429 `Retry-After` seconds).
  A 401 on an authenticated call clears the session and redirects to `/login?redirect=…` (`keepSession: true` opts out,
  e.g. a failed WebAuthn check); a 403 `mfa_setup_required` (admin/staff without 2FA) sends the user to
  `/profile?tab=security`.
- Types for every object are in `src/lib/api/types.ts` (money = cents, traffic = bytes, `Plan.transfer_enable` = GB).
- Reads: use (or add) a `xxxQuery()` factory in `src/lib/api/queries.ts` — `queryOptions` + `qk` key + forwarded `signal`.
- Writes: `useMutation` → on success `queryClient.invalidateQueries({ queryKey: qk.user.orders() })` (or `qk.admin.all`).
- TanStack Query 5.10x deprecates `ensureQueryData/fetchQuery/prefetchQuery` → use `queryClient.query(o)`
  (`{ ...o, staleTime: 'static' }` = ensure).
- **React Query only exists inside the `_user` / `_admin` areas** (lazy-loaded by `withQueryClient` in the layout
  `beforeLoad`, provided by `AppShell`). The public pages (landing/auth) use `lib/publicData.ts` + route loaders to stay
  inside the first-load budget — don't add `useQuery` there.

## Lists: `DataTable`

```tsx
import { createDataTableColumns, DataTable } from '@/components/data-table'
const col = createDataTableColumns<AdminUser>()          // module scope = stable reference
const columns = col.columns([
  col.accessor('email', { header: '邮箱', meta: { isPrimary: true } }),
  col.accessor('balance', { header: '余额', meta: { align: 'end' }, cell: (c) => <Money cents={c.getValue()} /> }),
])

<DataTable aria-label="用户" columns={columns}
  data={q.data?.items} total={q.data?.total} isLoading={q.isPending} isFetching={q.isFetching}
  page={search.page} pageSize={search.page_size} onPageChange={(page) => navigate({ search: (s) => ({ ...s, page }) })}
  sort={search.sort} onSortChange={(sort) => navigate({ search: (s) => ({ ...s, sort, page: 1 }) })}
  getRowId={(u) => String(u.id)} rowActions={(u) => <UserActions user={u} />} />
```

TanStack Table v9 (`useTable` + `tableFeatures({ rowSortingFeature, rowPaginationFeature })`, manual/server mode) rendered
with HeroUI `Table` (sortable headers, `renderEmptyState`, `Pagination` footer). Below `md` it renders a card list
(`meta.isPrimary` = card title, `meta.hideOnMobile`, `meta.label`, or pass `renderCard`). Skeleton rows while
`isLoading`, dimmed rows while `isFetching`. Keep page/sort/filters in the URL search params.

## Forms

Authenticated pages: **TanStack Form + HeroUI fields** via `useAppForm` (`@/components/form`):

```tsx
const form = useAppForm({ defaultValues: { email: '' }, onSubmit: async ({ value, formApi }) => {
  try { await adminApi.createUser(value) } catch (e) { setFormError(submitError(formApi, e)) }   // field errors → fields
} })
<form noValidate onSubmit={(e) => { e.preventDefault(); void form.handleSubmit() }}>
  <FormError message={formError} />
  <form.AppField name="email" validators={{ onBlur: ({ value }) => validateEmail(value) }}>
    {(f) => <f.TextField label="邮箱" type="email" autoComplete="email" isRequired />}
  </form.AppField>
  <form.AppForm><form.SubmitButton>保存</form.SubmitButton></form.AppForm>
</form>
```

Field components: `TextField` (types, `startContent`/`endContent`), `TextAreaField`, `NumberField` (plain numeric input,
no steppers: `unit="GB"` / `prefix="¥"` adornments, money as decimal units with `minimumFractionDigits: 2` — never cents),
`SelectField` (`options: {value,label,description?}[]`), `SwitchField` (Settings-style row: title + caption left, switch
right; `layout="inline"` for the old look), `TagsField` (string lists as chips). All render a visible 13 px label,
`name`/`id`, `isInvalid` + 13 px error text; server errors (`applyServerErrors`/`submitError`) clear as soon as the field
changes. `useAppForm` scrolls to and focuses the first invalid field after a failed submit (and after server field
errors) — never stealing focus the user already moved. Long forms: `<SaveBar dirty saving …/>`
(`@/components/form/SaveBar`, sticky glass capsule above the tab bar). Admin forms with runtime-defined fields use the
controlled twins in `features/admin/settings/inputs/Controlled.tsx` (same props, `unit`/`prefix` too).

Token field: `<TagInput label value onChange normalize? validate? suggestions?/>` (`@/components/form/TagInput`) —
Enter / comma / semicolon adds, paste splits a list, Backspace selects then removes the last chip, chips have remove
buttons, `suggestions` render as "+ item" chips. Fixed option sets (e.g. `telegram_events`) use `<ChipSelect>`
(`@/components/form/ChipSelect`, toggle chips with `aria-pressed`).

Admin settings (`features/admin/settings`): metadata-driven (`GET /admin/settings` `groups`), laid out by
`layout.ts` — sidebar icon per group, titled sections, dependent keys nested under their switch/select and hidden while
it is off, units per key. Unknown keys and groups still render (trailing section / generic group), so backend additions
never disappear; the `telegram` group adds test / webhook actions (`TelegramActions.tsx`).

Public auth pages use `NativeField` / `NativeButton` + `useLiteForm` instead (HeroUI's CSS classes on native elements, no
React Aria JS) — see "Performance".

## Feedback

- Toast: `import { toast } from '@/lib/toast'` → `toast(msg)`, `toast.success(msg)`, `toast.error(errorMessage(e))`
  (`danger` = alias), `toast.info`, `toast.warning`, `toast.promise(p, {loading, success, error})`, `toast.dismiss(id)`;
  options `{ description, timeout (ms, 0 = sticky), id }`. Own renderer (no HeroUI toast): Apple capsules (14 px radius,
  thick material + hairline + soft shadow, status symbol, 15 px medium text), spring in from the top on desktop / from
  the bottom above the tab bar on phones, 3.2–4 s auto-dismiss (paused on hover/focus/hidden tab), max 3 (a repeated
  message refreshes instead of stacking), dismiss button + Esc, `role="status"`, reduced-motion → fade. The store is a
  ~1 KB module in the entry; `components/feedback/ToastHost.tsx` (+ `toast.css`) loads lazily. **Never** import
  `toast` from `@heroui/react`.
- Confirm: `if (await confirm({ title, description, tone: 'danger' })) …` (`@/components/feedback/confirm`), or the
  controlled `<ConfirmDialog isOpen onOpenChange onConfirm={() => mutation.mutateAsync()} />` (pending state, keeps open on error).
- `errorMessage(e)` (`@/lib/errors`) → localized text for transport errors, server text for business errors.

## Shared components (`src/components`)

`PageShell` (Large Title 34/700 + 15 px subtitle, 1120 px max width, 32 px between sections), `Section` (titled group +
caption + card), `EmptyState` (ContentUnavailableView style — also for empty tables: `emptyState={<EmptyState compact icon={…} title={…} />}`), `SegmentedControl` / `SegmentedTabs` (below), `LoadingPage`/`Spinner`, `ErrorBoundary`, `RouteError`,
`NotFound`, `StatCard` (symbol badge + label, large tabular value; numbers count up after idle), `TrafficBar` and
`TrafficRing` (ARIA meters), `Money`/`Traffic`/`DateTime` (`Formatters.tsx`, Intl based; raw functions in `lib/format.ts`),
`CopyButton` (+ `lib/clipboard.ts`), `QRCode` (lazy `qrcode`, SVG), `Markdown` (lazy `marked` + `dompurify`),
`ThemeToggle`, `LocaleToggle`, `Logo`, `Captcha` (Turnstile / reCAPTCHA / hCaptcha script loaded only when enabled; the
backend CSP must allow the provider when it is on; `LazyCaptcha` defers it), `AriaProviders` (React Aria router + locale, rendered by
`AppShell`), `data-table/`, `form/`, `feedback/`, `layout/` (AppShell, AuthLayout, nav), `charts/` (SVG line/bar/sparkline),
`editor/` (Markdown editor), `native/` (`NativeField`, `NativeButton`), `icons/inline.tsx`, `auth/EmailCodeButton`.
`effects/` — ShinyText, GradientText, BlurText, SpotlightCard, CountUp (original CSS/rAF effects, GPL-3.0; always behind `WhenIdle`/`lazy`).

## Segmented control and tabs (`@/components/SegmentedControl`)

One iOS segmented control for **every** filter, mode switch and content tab — never HeroUI `Tabs` or a raw
`ToggleButtonGroup` for this (they stretch to the container and use a different pill).

```tsx
import { SegmentedControl, SegmentedTabs } from '@/components/SegmentedControl'

// Filter / mode switch → role=radiogroup (React Aria ToggleButtonGroup): one tab stop, ←/→/↑/↓ move AND select
// (wrapping, disabled items skipped), Home/End. Keep the value in the URL where it filters a list.
<SegmentedControl aria-label="订单状态" value={tab} onChange={(v) => navigate({ search: … })}
  items={[{ id: 'all', label: '全部' }, { id: '0', label: '待支付', count: 2 }, { id: '1', label: '开通中' }]} />
<SegmentedControl label="排序" size="sm" value={sort} onChange={setSort} items={…} />   // visible 13 px label

// Real content tabs → role=tablist + tabpanel (React Aria Tabs, automatic activation). Same look.
<SegmentedTabs value={tab} onChange={setTab}>
  <SegmentedTabs.List aria-label="个人中心" items={[{ id: 'account', label: '账户' }, …]} />
  <SegmentedTabs.Panel id="account">…</SegmentedTabs.Panel>   {/* shouldForceMount + data-[inert=true]:hidden ok */}
</SegmentedTabs>
```

- Items: `{ id, label, textValue?, icon?, count?, countTone?: 'accent', adornment?, isDisabled? }` (`count` = capsule,
  `accent` = tinted "needs attention"; `adornment` = status dot etc.). `size`: `md` 32 px segments (default), `sm` 28 px.
- Look (`app.css` `.gg-segmented*`, tokens `--segment`, `--segment-track`, `--segment-divider`, `--segment-shadow`): hug
  content (never stretched), 2 px inset track, 9 px radius, 13 px semibold labels, one white (dark `#636366`) pill that
  slides to the selection (CSS transform, measured; reduced motion → jump), hairline dividers that fade next to the
  pill / a hovered segment, pressed segment dims, pressed pill shrinks to 0.96. Too wide → it scrolls inside an
  edge-faded container and keeps the selection in view.
- Picks show at once even when the owner commits later (router search params): the control keeps the pick for up to
  2 s while `value` is still the old one. `SegmentedTabs optimistic={false}` when the owner may refuse/confirm first or
  renders only the committed panel (subscription profiles).
- Tests: single selection = `getByRole('radio', { name })`; tabs = `getByRole('tab')`. No `getAnimations` stub needed.

## i18n

- Dictionaries: `src/i18n/<locale>/<namespace>.json` (nested keys, `{param}` interpolation, `key_one/_other` plurals).
  Every locale must have the same keys (unit test). Default `zh-CN`, fallback `en`.
- Namespaces: `core` (always loaded: app name, a11y, theme, locale, errors), `common` (UI kit, loaded by the shells),
  `nav`, `auth`, `landing`. Add one per feature (`orders.json`, `admin-users.json`, …).
- `const t = useT(['orders', 'common'])` — suspends until loaded; `t('title')`, `t('common:actions.save')`.
  Preload in the route loader: `loadNamespaces(['orders'])`. Outside React: `t('core', 'errors.network')`.

## Design tokens (Apple HIG → HeroUI → Tailwind)

| use                  | classes / variables                                                                                                                                    |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| page / card / nested | `bg-canvas` (systemGroupedBackground), `bg-elevated` / `surface-card` (card + radius + shadow), `bg-elevated-secondary`, `inset-group`                 |
| text                 | `text-label`, `text-label-secondary` (AA-safe 80 %), `text-label-tertiary`, `text-label-blue/green/orange/red` (HIG increased-contrast, for text)      |
| raw system colors    | `bg-system-blue` … `text-system-gray-6` — **icons, dots, progress only** (they fail 4.5:1 as text)                                                     |
| fills / lines        | `bg-fill(-secondary/-tertiary/-quaternary)`, `border-hairline`, `hairline-t/-b`, `glass glass-border-b` (frosted bars)                                 |
| type                 | `text-large-title`, `text-title1/2/3`, `text-headline`, `text-body`, `text-callout`, `text-subhead`, `text-footnote`, `text-caption`, `text-caption2`  |
| radii / motion       | `rounded-card` 20 px, `rounded-button` 12 px, `rounded-input` 10 px, `ease-apple-out`, `animate-enter` (not on LCP elements)                           |
| micro-interactions   | `pressable` (tappable cards/rows: 0.98 press), `shimmer` (skeleton placeholders), `--ease-spring` (sheets) — app.css only                              |
| layout               | `max-w-page` (1120 px), `surface-card` (20 px radius, hairline + soft shadow), `inset-group` / `separated` (hairline rows; `[--separator-inset:20px]`) |
| controls             | `--field-h` 44 → 38 px (≥ md), `--field-fs` 17 → 15 px, `field-affix(--start)` units, `bg-selection` (tinted pill), `--material-thick` (toasts)        |
| HeroUI               | `bg-background`, `bg-surface`, `text-muted`, `bg-accent` (= `--tint` #006EF8, white text 4.58:1), `text-danger` … all mapped to the tokens above       |

Component skin (`styles/theme.css` global part + `styles/app.css`, which `AppShell` imports so public pages never load it),
all in the `gg-overrides` layer after HeroUI's `components` layer:

- **Buttons — four roles**: `primary` (filled tint), `secondary` (`--fill-secondary` grey, label text), `tertiary`
  (plain tint text), `danger` (red); `ghost` for chrome icons. 44/38 px (sm 36/32, lg 50/44), 17/15 px semibold, icon-only
  36 px; pressed = 0.98 scale + dim; disabled 40 %; every `isPending`/`pending` button gets an inline spinner.
- **Fields**: fill style (`--fill-tertiary`), 10 px radius, 2 px tint focus ring, placeholder `--label-tertiary`,
  invalid = red tint + ring; labels 13 px semibold, helper/error 13 px.
- **Switches** iOS proportions + systemGreen; **segments/tabs** `SegmentedControl` / `SegmentedTabs` (above); **menus/selects** 14 px popovers with grey
  highlight; **tables** hairline rows, no zebra/dividers, sticky header inside a scroll view (`gg-table`), end-aligned
  numbers tabular; **dialogs** 20 px radius, Title3 headings, bottom sheets with a grabber on phones (`FormDrawer`:
  floating inspector on desktop, `Drawer.Handle` sheet on phones); **chips** capsules; **avatars** Contacts-style
  monogram gradient; lucide icons at 1.75 stroke.
- Navigation: sidebar 15 px items / 20 px icons, tinted pill for the active item, admin sections (`nav.ts` `section`:
  运营 / 内容 / 系统), collapsible; glass top bar; phone tab bar (`data-tabbar`, 50 px + safe area).

No hex colors in components. Dark mode follows the OS; `useUiStore().setTheme('light'|'dark'|'system')` forces it via
`<html data-theme>`; `dark:` utilities honour both. Links that look like buttons: `buttonClass({ variant, size })`.

## Performance rules (AGENTS §6)

- **First-load budget: ≤ 120 KB gzip JS** for landing/login. Current (2026-10-22): entry graph 111.8 KB; with route chunk,
  dictionaries and boot module landing ≈ 121 KB, login ≈ 122 KB, register/forgot ≈ 120 KB. The public pages are
  prerendered, so this JS no longer gates their first paint (it loads after it).
  Fixed cost: React 19 (67 KB) + TanStack Router (26 KB). Check with `pnpm build` (sum the chunks a page loads).
- Public pages use native elements with HeroUI CSS classes (`NativeButton`, `NativeField`, `buttonClass`, inline icons in
  `components/icons/inline.tsx`) and no React Query; HeroUI React components, React Aria, lucide and React Query load with
  the authenticated shells.
- HeroUI CSS is per component (`scripts/heroui-css.ts` attaches each component's stylesheet to its JS module); in a
  native element use `import 'heroui-css:button'`. The global CSS stays ≈ 11 KB gzip.
- Route-level code splitting is automatic; lazy-load anything heavy (charts, editors, Markdown, QR, effects) and start
  animations after idle (`useAnimationsReady`, `WhenIdle`). Give async content a fixed-size placeholder (CLS 0).
- Vendor groups (`vite.config.ts` → `rolldownOptions.output.codeSplitting.groups`): `react`, `tanstack`, `tanstack-query`,
  `heroui` (entries-aware), plus app groups. **No new dependency without need** — check the bundle impact.
- Lighthouse: `pnpm lh` asserts 100/100/100/100 on `/`, `/login`, `/register`, `/forgot` with the desktop AND the mobile
  preset (served like the Go server: prerendered pages, brotli + immutable assets, fixture API).
- Prerendering (`scripts/prerender.mjs`, part of `pnpm build`): headless Chrome renders each public route once
  (`window.__GG_PRERENDER__`: main.tsx waits for the route and hands over the first committed markup) and writes
  `dist/<route>/index.html`; the plain shell becomes `dist/spa.html` (deep links). Snapshot pages load only CSS and
  `assets/boot-*.js`, which imports the app after the snapshot's LCP entry — Lighthouse's simulation counts every
  request started before the first paint, and React replacing nodes before Chrome reports them moves LCP to the swap.
  The client never hydrates (a client snapshot has client `useId`s): it renders the complete route in its first commit
  (`router.load()` first), carries typed values over (`lib/snapshot.ts`) and replaces the snapshot in one frame.
  Rules: keep the first commit of a public route free of Suspense fallbacks and data that differs from a fresh install
  (the fixture `scripts/fixtures/guest-config.json` = backend defaults); anything visible in the first phone viewport
  must be in the route chunk (a lazily revealed block becomes a later LCP candidate — hence `-landing/Features.tsx`).
- Route loaders and route-option code stay in the entry chunk: import query factories / API modules lazily there and
  never import the `@/lib/api` barrel in a route file (the code splitter leaves a bare import behind → React Query in
  every page). Check `grep -o 'import"./[^"]*"' dist/assets/index-*.js` after adding a loader.

## Assumptions about the backend

- Admin "login as" URL is `<app_url>/login?token=<one-time token>` (`POST /admin/users/:id/login-token` → `{url}`; the login
  page exchanges it with `POST /passport/login/token`).
- Invite links: `<app_url>/register?code=<invite code>`. Payment return: gateways go to
  `/api/v1/guest/payment/return/<trade_no>`, which 302s to `/orders/<trade_no>`.
- `/.well-known/change-password` and `/.well-known/passkey-endpoints` point password managers at `/profile?tab=security`.
- Static server (`internal/static`): `<path>/index.html` (prerendered) when present, else SPA fallback to `spa.html` →
  `index.html` (no-cache); precompressed `.br`/`.gz` and `immutable` caching for hashed assets.

## Tooling notes

- `src/routeTree.gen.ts` is generated and git-ignored; `pnpm typecheck|lint|test` regenerate it first
  (`pnpm routes` = resolve the Vite config, which runs the router generator).
- TypeScript 7 (`tsc -b`) has no compiler API; `.pnpmfile.cjs` gives typescript-eslint its own TypeScript 6.0 so
  type-aware lint works (see `docs/FRONTEND.md` §1).
- `pnpm-workspace.yaml` (pnpm ≥ 10 settings; standalone project). New packages younger than pnpm's minimum release age are
  held back (`minimumReleaseAgeExclude` lists the exceptions). Node ≥ 22 (`engines`).
