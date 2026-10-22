# GG Bond — web

React 19 + TypeScript 7 + Vite 8 (Rolldown) + Tailwind CSS v4 + **HeroUI v3** + TanStack (Router, Query, Table, Form,
Virtual) + zustand. Apple HIG design tokens. Everything below is a **convention for feature agents** — read it together
with `AGENTS.md`, `MISTAKES.md` and `docs/FRONTEND.md` (verified library cheat sheet).

```bash
pnpm install
pnpm dev                 # http://localhost:5173, /api → http://localhost:8080 (GGBOARD_API=… to override)
pnpm typecheck && pnpm lint && pnpm test && pnpm build && pnpm lh   # quality gates — all must pass
pnpm build               # vite build + scripts/prerender.mjs (public routes; needs Chrome, else skipped)
pnpm preview:mock        # serve dist/ like the Go server, with fixture API (demo login: any email/password)
pnpm lh                  # build, then Lighthouse desktop AND mobile on /, /login, /register, /forgot (asserts 100s)
LH_BASE=http://127.0.0.1:18090 pnpm lh:mobile           # same against a running Go binary
LH_BASE=… LH_TARGET=auth pnpm lh:desktop                 # signed-in /dashboard, /subscription (LH_USER/LH_PASSWORD)
BASE=http://127.0.0.1:18090 pnpm ui:smoke --axe          # crawl every page: console/network errors, i18n leaks, overflow, axe
BASE=http://127.0.0.1:18090 node scripts/screenshots.mjs # README screenshots → ../docs/screenshots
pnpm format              # prettier
```

## Layout

```
src/
  main.tsx                 bootstrap: theme, <html lang>, `core` dictionary, router
  router.tsx               createRouter (preload on intent, default pending/error/404, 401 → /login)
  routes/                  file-based routes (TanStack Router plugin → routeTree.gen.ts, do not edit)
    __root.tsx             tiny root: Suspense + toast region + 404
    index.tsx              landing (hero) · -landing/ = lazy below-the-fold sections ("-" = ignored by the generator)
    login.tsx register.tsx forgot.tsx
    _user.tsx              pathless layout: /dashboard /shop /orders /orders/$tradeNo /subscription /invite /tickets …
    _user/*.tsx            user pages (placeholders → replace the component body)
    _admin.tsx             pathless layout (is_admin || is_staff) for /admin/*
    _admin/admin/*.tsx     admin pages (admin-only pages keep `beforeLoad: requireAdmin`)
  lib/api/                 client.ts, types.ts, guest.ts, passport.ts, user.ts, admin.ts, queries.ts
  lib/                     guards, format, toast, motion, theme, errors, button, publicData, queryClient, validators
  stores/                  auth.ts (token/user), ui.ts (theme/locale/sidebar) — persisted to localStorage
  i18n/                    index.ts + <locale>/<namespace>.json
  components/              shared UI (see below); reactbits/ (lazy animated text/cards)
  styles/                  index.css (entry), theme.css (Apple tokens), utilities.css
scripts/                   heroui-css.ts (per-component CSS), preview.ts (prod-like preview + mock API), fixtures/
```

## Adding a page

1. Create the route file, e.g. `src/routes/_user/orders/index.tsx` (URL `/orders`) or
   `src/routes/_admin/admin/users.tsx` (URL `/admin/users`). The dev server/build regenerates `routeTree.gen.ts`.
2. Export **only** `Route` from a route file (anything else breaks code splitting). Put helpers next to it in a
   `-something/` folder or `-columns.tsx` file (the `-` prefix is ignored by the generator).
3. Wrap the body in `<PageShell title=… description=… actions=…>`; it renders the single `<h1>` and sets `document.title`.
4. Admin-only pages: `beforeLoad: requireAdmin`. Staff pages (tickets/notices/knowledge): nothing extra.
5. Navigation entries live in `src/components/layout/nav.ts` (labels in `i18n/*/nav.json`).

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

- **Never call `fetch` directly.** Endpoint functions for every route in `docs/API.md` live in `src/lib/api/{guest,passport,user,admin}.ts`:
  `import { userApi, adminApi } from '@/lib/api'` → `userApi.orders({ page })`, `adminApi.updateUser(id, patch)`.
- The client (`lib/api/client.ts`) unwraps `{data}` → `T` and `{data,total}` → `Page<T> = { items, total }`, adds
  `Authorization: Bearer`, times out after 15 s and throws `ApiError { status, code, message, errors, fieldError(f) }`.
  A 401 on an authenticated call clears the session and redirects to `/login?redirect=…`.
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

`PageShell`, `EmptyState`, `ComingSoon`, `LoadingPage`/`Spinner`, `StatCard` (numbers count up after idle),
`TrafficBar` (ARIA meter), `Money`/`Traffic`/`DateTime` (`Formatters.tsx`, Intl based; raw functions in `lib/format.ts`),
`CopyButton` (+ `lib/clipboard.ts`), `QRCode` (lazy `qrcode`, SVG), `Markdown` (lazy `marked` + `dompurify`),
`ThemeToggle`, `LocaleToggle`, `Logo`, `Captcha` (Turnstile / reCAPTCHA / hCaptcha script loaded only when enabled; the
backend CSP must allow the provider when it is on), `data-table/`, `form/`, `feedback/`, `layout/` (AppShell, AuthLayout, nav).
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

- **First-load budget: ≤ 120 KB gzip JS** for landing/login. Current (2026-10-22): entry graph 111.8 KB; with route chunk,
  dictionaries and boot module landing ≈ 121 KB, login ≈ 122 KB, register/forgot ≈ 120 KB. The public pages are
  prerendered, so this JS no longer gates their first paint (it loads after it).
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

- Admin "login as" URL is `/login?token=<one-time token>` (handled by the login page → `POST /passport/login/token`).
- Invite links: `/register?code=<invite code>`. Payment return: `/orders/<trade_no>` (route exists).
- Static server: `<path>/index.html` (prerendered) when present, else SPA fallback to `spa.html` → `index.html` (no-cache); `.br`/`.gz` + `immutable` for `/assets/*`.

## Tooling notes

- `src/routeTree.gen.ts` is generated and git-ignored; `pnpm typecheck|lint|test` regenerate it first
  (`pnpm routes` = resolve the Vite config, which runs the router generator).

- TypeScript 7 (`tsc -b`) has no compiler API; `.pnpmfile.cjs` gives typescript-eslint its own TypeScript 6.0 so
  type-aware lint works (equivalent to the alias documented in `docs/FRONTEND.md` §1).
- `pnpm-workspace.yaml` (pnpm ≥ 10 settings). New packages younger than pnpm's minimum release age are held back.
