# GG Bond — web

React 19 + TypeScript 7 + Vite 8 (Rolldown) + Tailwind CSS v4 + **HeroUI v3** + TanStack (Router, Query, Table, Form,
Virtual) + zustand. Apple HIG design tokens. Everything below is a **convention for feature agents** — read it together
with `AGENTS.md`, `MISTAKES.md` and `docs/FRONTEND.md` (verified library cheat sheet).

```bash
pnpm install
pnpm dev                 # http://localhost:5173, /api and /s/ → http://localhost:8080 (GGBOARD_API=… to override)
pnpm typecheck && pnpm lint && pnpm test && pnpm build && pnpm lh   # quality gates — all must pass
pnpm routes              # regenerate src/routeTree.gen.ts (typecheck/lint/test do it first)
pnpm preview:mock        # serve dist/ like the Go server + fixture API (demo login: any email/password)
pnpm lh:mobile           # Lighthouse with mobile emulation (informational, see "Performance")
pnpm format              # prettier (format:check in CI)
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

Field components: `TextField` (types, `startContent`/`endContent`), `TextAreaField`, `NumberField`, `SelectField`
(`options: {value,label}[]`), `SwitchField` (HeroUI ≥ 3.2 anatomy). All render a visible `<Label>`, `name`/`id`, `isInvalid` +
`<FieldError>`; server errors (`applyServerErrors`/`submitError`) clear as soon as the field changes.

Public auth pages use `NativeField` / `NativeButton` + `useLiteForm` instead (HeroUI's CSS classes on native elements, no
React Aria JS) — see "Performance".

## Feedback

- Toast: `import { toast } from '@/lib/toast'` → `toast.success(msg)`, `toast.danger(errorMessage(e))`, `toast.promise(p, {…})`.
  Lazy HeroUI toast — **never** import `toast` from `@heroui/react`.
- Confirm: `if (await confirm({ title, description, tone: 'danger' })) …` (`@/components/feedback/confirm`), or the
  controlled `<ConfirmDialog isOpen onOpenChange onConfirm={() => mutation.mutateAsync()} />` (pending state, keeps open on error).
- `errorMessage(e)` (`@/lib/errors`) → localized text for transport errors, server text for business errors.

## Shared components (`src/components`)

`PageShell`, `EmptyState`, `LoadingPage`/`Spinner`, `ErrorBoundary`, `RouteError`, `NotFound`, `StatCard` (numbers count up after idle),
`TrafficBar` (ARIA meter), `Money`/`Traffic`/`DateTime` (`Formatters.tsx`, Intl based; raw functions in `lib/format.ts`),
`CopyButton` (+ `lib/clipboard.ts`), `QRCode` (lazy `qrcode`, SVG), `Markdown` (lazy `marked` + `dompurify`),
`ThemeToggle`, `LocaleToggle`, `Logo`, `Captcha` (Turnstile / reCAPTCHA / hCaptcha script loaded only when enabled; the
backend CSP must allow the provider when it is on; `LazyCaptcha` defers it), `AriaProviders` (React Aria router + locale, rendered by
`AppShell`), `data-table/`, `form/`, `feedback/`, `layout/` (AppShell, AuthLayout, nav), `charts/` (SVG line/bar/sparkline),
`editor/` (Markdown editor), `native/` (`NativeField`, `NativeButton`), `icons/inline.tsx`, `auth/EmailCodeButton`.
`reactbits/` — ShinyText, GradientText, BlurText, SpotlightCard, CountUp (React Bits, _MIT + Commons Clause_: use inside
the app is fine, don't extract them). Always via the lazy exports + `<WhenIdle fallback={same-size static}>`.

## i18n

- Dictionaries: `src/i18n/<locale>/<namespace>.json` (nested keys, `{param}` interpolation, `key_one/_other` plurals).
  Every locale must have the same keys (unit test). Default `zh-CN`, fallback `en`.
- Namespaces: `core` (always loaded: app name, a11y, theme, locale, errors), `common` (UI kit, loaded by the shells),
  `nav`, `auth`, `landing`. Add one per feature (`orders.json`, `admin-users.json`, …).
- `const t = useT(['orders', 'common'])` — suspends until loaded; `t('title')`, `t('common:actions.save')`.
  Preload in the route loader: `loadNamespaces(['orders'])`. Outside React: `t('core', 'errors.network')`.

## Design tokens (Apple HIG → HeroUI → Tailwind)

| use                  | classes / variables                                                                                                                                   |
| -------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| page / card / nested | `bg-canvas` (systemGroupedBackground), `bg-elevated` / `surface-card` (card + radius + shadow), `bg-elevated-secondary`, `inset-group`                |
| text                 | `text-label`, `text-label-secondary` (AA-safe 80 %), `text-label-tertiary`, `text-label-blue/green/orange/red` (HIG increased-contrast, for text)     |
| raw system colors    | `bg-system-blue` … `text-system-gray-6` — **icons, dots, progress only** (they fail 4.5:1 as text)                                                    |
| fills / lines        | `bg-fill(-secondary/-tertiary/-quaternary)`, `border-hairline`, `hairline-t/-b`, `glass glass-border-b` (frosted bars)                                |
| type                 | `text-large-title`, `text-title1/2/3`, `text-headline`, `text-body`, `text-callout`, `text-subhead`, `text-footnote`, `text-caption`, `text-caption2` |
| radii / motion       | `rounded-card` 18 px, `rounded-button` 12 px, `rounded-input` 10 px, `ease-apple-out`, `animate-enter` (not on LCP elements)                          |
| HeroUI               | `bg-background`, `bg-surface`, `text-muted`, `bg-accent` (= `--tint` #006EF8, white text 4.58:1), `text-danger` … all mapped to the tokens above      |

No hex colors in components. Dark mode follows the OS; `useUiStore().setTheme('light'|'dark'|'system')` forces it via
`<html data-theme>`; `dark:` utilities honour both. Links that look like buttons: `buttonClass({ variant, size })`.

## Performance rules (AGENTS §6)

- **First-load budget: ≤ 120 KB gzip JS** for landing/login. Last measured at merge time: landing ≈ 109 KB, login ≈ 111 KB
  (incl. dictionaries); re-measure after dependency changes.
  Fixed cost: React 19 (67 KB) + TanStack Router (26 KB). Check with `pnpm build` (sum the chunks a page loads).
- Public pages use native elements with HeroUI CSS classes (`NativeButton`, `NativeField`, `buttonClass`, inline icons in
  `components/icons/inline.tsx`) and no React Query; HeroUI React components, React Aria, lucide and React Query load with
  the authenticated shells.
- HeroUI CSS is per component (`scripts/heroui-css.ts` attaches each component's stylesheet to its JS module); in a
  native element use `import 'heroui-css:button'`. The global CSS stays ≈ 11 KB gzip.
- Route-level code splitting is automatic; lazy-load anything heavy (charts, editors, Markdown, QR, reactbits) and start
  animations after idle (`useAnimationsReady`, `WhenIdle`). Give async content a fixed-size placeholder (CLS 0).
- Vendor groups (`vite.config.ts` → `rolldownOptions.output.codeSplitting.groups`): `react`, `tanstack`, `tanstack-query`,
  `heroui` (entries-aware), plus app groups. **No new dependency without need** — check the bundle impact.
- Lighthouse: `pnpm lh` (desktop preset) asserts 100/100/100/100 on `/`, `/login`, `/register` (served like the Go server:
  brotli + immutable assets, fixture API). Mobile emulation (`pnpm lh:mobile`) scores 94–95 performance with
  a11y/BP/SEO 100: an SPA's FCP/LCP on simulated slow 4G is bound by the framework JS; reaching 100 there needs
  prerendered HTML for the public routes (future work).

## Assumptions about the backend

- Admin "login as" URL is `<app_url>/login?token=<one-time token>` (`POST /admin/users/:id/login-token` → `{url}`; the login
  page exchanges it with `POST /passport/login/token`).
- Invite links: `<app_url>/register?code=<invite code>`. Payment return: gateways go to
  `/api/v1/guest/payment/return/<trade_no>`, which 302s to `/orders/<trade_no>`.
- `/.well-known/change-password` and `/.well-known/passkey-endpoints` point password managers at `/profile?tab=security`.
- Static server (`internal/static`): SPA fallback to `index.html` (no-cache), precompressed `.br`/`.gz` and `immutable`
  caching for hashed assets.

## Tooling notes

- `src/routeTree.gen.ts` is generated and git-ignored; `pnpm typecheck|lint|test` regenerate it first
  (`pnpm routes` = resolve the Vite config, which runs the router generator).
- TypeScript 7 (`tsc -b`) has no compiler API; `.pnpmfile.cjs` gives typescript-eslint its own TypeScript 6.0 so
  type-aware lint works (see `docs/FRONTEND.md` §1).
- `pnpm-workspace.yaml` (pnpm ≥ 10 settings; standalone project). New packages younger than pnpm's minimum release age are
  held back (`minimumReleaseAgeExclude` lists the exceptions). Node ≥ 22 (`engines`).
