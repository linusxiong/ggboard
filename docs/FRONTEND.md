# FRONTEND.md — verified cheat sheet for `web/`

Read this before you write a page. Every library API here was checked on **2026-10-21** against the official docs and/or the published `.d.ts`
of the exact versions below. Each section cites its sources. If this file conflicts with your memory, **this file wins**. If it conflicts with `AGENTS.md`,
AGENTS wins: raise the conflict and fix this file. **Use the building blocks in §0** and don't re-implement them; the `web/src` layout,
page recipe and conventions are in [`web/README.md`](../web/README.md). Data shapes are defined in `docs/API.md` and `web/src/lib/api/types.ts`.
Versions below are the ones resolved in `web/pnpm-lock.yaml`.

| package | version | package | version |
|---|---|---|---|
| `@heroui/react` / `@heroui/styles` | 3.2.6 / 3.2.6 | `@tanstack/react-table` | 9.3.1 |
| `tailwindcss` / `@tailwindcss/vite` | 4.3.3 | `@tanstack/react-form` | 1.33.5 |
| `vite` / `@vitejs/plugin-react` | 8.3.5 / 6.1.2 | `@tanstack/react-virtual` | 3.14.21 |
| `react` / `react-dom` | 19.3.1 | `zustand` | 5.0.15 |
| `@tanstack/react-router` / `router-plugin` | 1.170.45 / 1.168.47 | `motion` | 14.4.0 |
| `@tanstack/react-query` | 5.104.5 | `lucide-react` | 1.60.0 |
| `typescript` (lint tooling: 6.0.3, §1) | 7.0.2 | `@simplewebauthn/browser` | 14.0.0 |
| `vite-plugin-compression2` / `@lhci/cli` | 2.5.3 / 0.15.1 | `react-aria-components` / `react-aria` | 1.22.1 / 3.53.1 |
| `marked` / `dompurify` | 18.1.1 / 3.4.17 | `qrcode` | 1.5.4 |
| `vitest` / `jsdom` | 5.0.5 / 30.1.3 | `eslint` / `typescript-eslint` | 10.13.0 / 8.72.0 |

There is **no schema library** (no Zod): search params and form fields are validated with plain functions
(`validateSearch` functions, `@/lib/validators`).

---

## 0. Building blocks (read each file's header comment)

| need | use | notes |
|---|---|---|
| HTTP | `api.get/list/post/put/patch/delete` from `@/lib/api`. Endpoint modules: `adminApi`, `userApi`, `guestApi`, `passportApi` | unwraps `{data}`. `api.list` → `Page<T> = { items, total }`. Throws `ApiError { status, code, errors, retryAfter, fieldError(f), isValidation, isNetwork }`. 401 → session cleared + `/login?redirect=…`; 403 `mfa_setup_required` → `/profile?tab=security` |
| Query keys / options | `qk` + `xxxQuery()` factories in `@/lib/api/queries` (`meQuery()`, `adminUsersQuery(q)`, …) | hierarchical: invalidate `qk.admin.all`, `qk.user.all`, … |
| Router | `src/router.tsx` (`defaultPreload: 'intent'`, `defaultPreloadStaleTime: 0`, `scrollRestoration`, default pending/error/notFound) | `__root.tsx` only holds Suspense, the lazy toast region and the 404. React Query and React Aria providers (`AriaProviders`) live in `AppShell` (authenticated areas) |
| Guards | `requireAuth`, `requireStaff`, `requireAdmin`, `redirectIfAuthenticated`, `safeRedirect`, `homeFor`, `withQueryClient` from `@/lib/guards` | pass them straight to `beforeLoad`; the `_user`/`_admin` layouts use `withQueryClient(args, requireAuth\|requireStaff)` to lazy-load React Query into the route context |
| Tables | `DataTable`, `createDataTableColumns<T>()`, `dataTableFeatures`, `parseSort`, `toSortParam`, `pageWindow` from `@/components/data-table` | server pagination and sort, mobile cards, skeletons |
| Forms | `useAppForm`, `withForm`, `withFieldGroup`, `FormError`, `applyServerErrors` from `@/components/form`; `submitError` from `@/components/form/submitError` | field comps: `TextField`, `TextAreaField`, `NumberField`, `SelectField`, `SwitchField`, plus `SubmitButton` |
| Public auth forms | `NativeField`, `NativeButton` (`@/components/native/*`), `useLiteForm`, `useFinishAuth`, `useCooldown` (`@/hooks/*`), `@/lib/validators` | native elements with HeroUI CSS classes and no React Aria JS (first-load budget) |
| Toast / confirm | `toast` from `@/lib/toast` (lazy HeroUI toast), `confirm()` from `@/components/feedback/confirm`, `ConfirmDialog` from `@/components/feedback/ConfirmDialog` | **never** import `toast` from `@heroui/react` directly, because that loads it eagerly |
| Theme / locale | `useUiStore` (`theme`, `setTheme`, `cycleTheme`, `locale`), `@/lib/theme` (`applyTheme`, `useResolvedTheme`), `ThemeToggle` | **do not** use HeroUI `useTheme` (see §3.1) |
| Auth state | `useAuthStore` (`token`, `user`, `hydrated`, `login(token,user)`, `setUser`, `logout`), `isAdmin(u)`, `isStaff(u)` | persisted under `gg-auth` |
| Links that look like buttons | `buttonClass({ variant, size })` from `@/lib/button` | `<Link to="/plans" className={buttonClass()}>` |
| Motion helpers | `onIdle`, `useIdle`, `useReducedMotion`, `useAnimationsReady` from `@/lib/motion` | start animations only after idle |
| effects | `@/components/effects` (lazy BlurText, CountUp, GradientText, ShinyText, SpotlightCard + `WhenIdle`) | original code (see §11) |
| i18n | `useT(ns)`, `loadNamespaces([...])`, `t(ns, key)` from `@/i18n` | dictionaries in `src/i18n/<locale>/<namespace>.json` |
| Tokens | `src/styles/theme.css` (HIG tokens → HeroUI vars → Tailwind) | see §2 for utility names |

---

## 1. Install and TypeScript 7

Sources: <https://heroui.com/en/docs/react/getting-started/quick-start> · <https://tailwindcss.com/docs/installation/using-vite> ·
<https://tanstack.com/router/latest/docs/installation/with-vite> · <https://devblogs.microsoft.com/typescript/announcing-typescript-7-0/> ·
`npm view @heroui/react peerDependencies` · `npm view typescript-eslint peerDependencies` · Vite `react-ts` template `tsconfig.app.json`

* HeroUI's peers (`react-aria`, `react-aria-components`, `@react-aria/ssr`, `@react-aria/utils`, `@internationalized/date`, `tailwindcss>=4`, `react>=19`)
  have **no** `peerDependenciesMeta`. They are mandatory and must stay in `web/package.json`. `@internationalized/date` became a peer in 3.2.6.
* **TS 7 ships no compiler API.** `typescript-eslint@8.72` declares `typescript: >=4.8.4 <6.1.0`. The TS 7 announcement documents an alias
  (`"typescript": "npm:@typescript/typescript6@^6.0.2"`). We solve it in `web/.pnpmfile.cjs` instead: a `readPackage` hook gives
  `typescript-eslint`, `@typescript-eslint/*` and `ts-api-utils` their own `typescript@6.0.3`, while the project (`tsc -b`) uses TS 7.0.2.
* TS 7 hard errors: `baseUrl`, `moduleResolution: node|node10|classic`, `target: es5`, `module: amd|umd|system|none`, `esModuleInterop:false`.
  New defaults: `strict: true`, `types: []` (so `"types": ["vite/client"]` is required for `import.meta.env`, `*.css` and `*.svg`), `rootDir: ./`,
  `noUncheckedSideEffectImports: true`.
* `web/tsconfig.app.json` also enables `verbatimModuleSyntax` (write `import type { X }`), `erasableSyntaxOnly` (no `enum`, `namespace` or
  constructor parameter properties), and **`noUncheckedIndexedAccess`**: `arr[i]` has type `T | undefined`, so guard it.
* Theme bootstrap: `syncTheme()` runs in `main.tsx` before render, and the "system" theme is pure CSS (`prefers-color-scheme`). A *forced* theme still
  flashes for one frame until the module executes. To remove that flash, add an inline pre-paint script that reads `gg-ui` and sets `data-theme`. The CSP then needs its
  `'sha256-…'` hash (with nonces, React Aria reads `<meta property="csp-nonce">`, per the HeroUI frameworks page).

---

## 2. Apple HIG tokens → HeroUI variables → Tailwind utilities

Sources: AGENTS.md §5 · HIG Color data <https://developer.apple.com/tutorials/data/design/human-interface-guidelines/color.json> (swatch values) ·
iOS label, fill and separator values from Flutter `CupertinoColors` (<https://github.com/flutter/flutter/blob/master/packages/flutter/lib/src/cupertino/colors.dart>) ·
<https://heroui.com/en/docs/react/getting-started/theming> and `/colors` · <https://tailwindcss.com/docs/theme> · <https://tailwindcss.com/docs/font-size>

| HIG name | light | dark |
|---|---|---|
| systemBlue / Green / Orange / Red (AGENTS) | `#007AFF` `#34C759` `#FF9500` `#FF3B30` | `#0A84FF` `#30D158` `#FF9F0A` `#FF453A` |
| …increased contrast (blue / green / orange / red) | `#0040DD` `#248A3D` `#C93400` `#D70015` | `#409CFF` `#30DB5B` `#FFB340` `#FF6961` |
| systemGray, Gray2…Gray6 | `#8E8E93 #AEAEB2 #C7C7CC #D1D1D6 #E5E5EA #F2F2F7` | `#8E8E93 #636366 #48484A #3A3A3C #2C2C2E #1C1C1E` |
| systemGroupedBackground / secondary / tertiary | `#F2F2F7` / `#FFFFFF` / `#F2F2F7` | `#000000` / `#1C1C1E` / `#2C2C2E` |
| systemBackground / secondary / tertiary | `#FFFFFF` / `#F2F2F7` / `#FFFFFF` | `#000000` / `#1C1C1E` / `#2C2C2E` |
| label / secondaryLabel / tertiaryLabel (= placeholder) / quaternary | `#000` / `rgba(60,60,67,.6)` / `.3` / `.18` | `#FFF` / `rgba(235,235,245,.6)` / `.3` / `.16` |
| separator / opaqueSeparator | `rgba(60,60,67,.29)` / `#C6C6C8` | `rgba(84,84,88,.6)` / `#38383A` |
| systemFill / secondary / tertiary / quaternary | `rgba(120,120,128,.2)` / `.16` / `rgba(118,118,128,.12)` / `rgba(116,116,128,.08)` | `rgba(120,120,128,.36)` / `.32` / `rgba(118,118,128,.24)` / `.18` |

> ⚠️ **Contrast vs Lighthouse a11y = 100** (normal text needs ≥ 4.5:1). Measured ratios for raw HIG values:
>
> * Failing: white on `#007AFF` **4.02**, on `#0A84FF` **3.65**, on `#FF3B30` **3.55**, on `#34C759` **2.22**; 60% `secondaryLabel` on white **3.44**; `systemGray` text on white **3.26**.
>
> That is why `theme.css` uses passing values:
>
> * `--tint` `#006EF8` with white **4.58** (very thin margin: re-measure before you touch it). Dark `#076EF4` gives **4.62**.
> * `--label-secondary` at 80% gives **5.97** on white and **5.59** on `#F2F2F7`. Dark 65% gives **6.79** on `#1C1C1E`.
> * `--label-blue` `#0062CC` gives **5.8**, `--label-green` `#1E7434` **5.83**, `--label-red` `#D70015` **5.38** (all on white). Dark `#409CFF` gives **6.01** on `#1C1C1E`.
> * `--success`/`--warning` take black foregrounds (**9.46**/**9.55**).
>
> **Rule:** text, and fills that sit behind text, use the `label-*`/`tint` tokens. The raw `system-*` colors are for icons, dots and progress fills only.
> FYI: the HIG page changed its system colors in June 2025 (blue `#0088FF`/`#0091FF`, red `#FF383C`/`#FF4245`). AGENTS keeps the classic values.

**HeroUI variable names** (exact, from `@heroui/styles` `themes/default/variables.css`): `--background --foreground --surface --surface-foreground
--surface-secondary --surface-tertiary --overlay --overlay-foreground --muted --default --default-foreground --accent --accent-foreground --success(-foreground)
--warning(-foreground) --danger(-foreground) --segment --border --separator --focus --link --backdrop --field-background --field-foreground --field-placeholder
--field-border --radius --field-radius --border-width --field-border-width --surface-shadow --overlay-shadow --field-shadow --skeleton-animation --tooltip-delay`.
HeroUI computes `*-hover`, `*-soft`, `*-soft-foreground` and `--separator-secondary|tertiary` itself via `color-mix()`. Its bridge (`themes/shared/theme.css`)
maps all of these to Tailwind as `--color-*` (`bg-surface`, `text-muted`, `bg-accent-soft`, …), plus `--radius-xs…4xl` (multiples of `--radius`), `--ease-out-fluid`,
`shadow-surface` and `shadow-overlay`.

**Mechanics** (all verified, and all used by `theme.css`):

```css
@import "tailwindcss";          /* 1st — then */
@import "@heroui/styles";       /* 2nd: order matters */
:root { --accent: …; }          /* UNLAYERED declarations beat HeroUI's layered defaults */
@theme inline { --color-label-secondary: var(--label-secondary); }   /* inline: utility uses the live var → follows dark mode */
@theme { --text-title2: 1.375rem; --text-title2--line-height: 1.75rem; --text-title2--font-weight: 700; --text-title2--letter-spacing: .016em; }
@layer components { .button { border-radius: var(--radius-button); } }    /* HeroUI renders BEM classes: .button, .button--primary, .card … */
```

Utilities `theme.css` provides today:

* Backgrounds: `bg-canvas` (grouped bg), `bg-elevated`, `bg-elevated-secondary`, `bg-fill(-secondary|-tertiary|-quaternary)`, `bg-tint`, `bg-glass`.
* Text: `text-label`, `text-label-secondary|tertiary|quaternary`, `text-label-blue|green|orange|red`.
* Borders: `border-hairline(-opaque)`.
* Raw system colors: `text-system-blue` … `text-system-gray-6` (icons only).
* Type: `text-large-title`, `text-title1|2|3`, `text-headline`, `text-body`, `text-callout`, `text-subhead`, `text-footnote`, `text-caption`, `text-caption2`.
* Radii, shadows, easing: `rounded-card|card-lg|button|input`, `shadow-card`, `shadow-float`, `ease-apple-out`.
* Fonts: `font-display`, `font-mono`.
* Custom `@utility` classes: `surface-card`, `inset-group`, `glass`, `glass-border-t|b`, `hairline`, `hairline-t|b`, `animate-enter`,
  `no-scrollbar`, `pt-safe`, `pb-safe`, `tabular`.

Do **not** set `--color-*: initial`, because it wipes HeroUI's bridge. **No hex values in components.** Inside a Card or Surface, give inputs `variant="secondary"` (HeroUI docs).

---

## 3. HeroUI v3 (`@heroui/react` 3.2.6)

Sources: `https://heroui.com/en/docs/react/components/{button,text-field,input,number-field,text-area,select,checkbox,switch,radio-group,card,modal,
drawer,alert-dialog,tabs,chip,badge,avatar,dropdown,table,tooltip,progress-bar,skeleton,toast,alert,form,link,breadcrumbs,pagination,accordion,popover,
date-picker}` (append `.mdx` to any docs URL to get the raw source) · `/getting-started/{dark-mode,theming,frameworks}` · `/releases/v3-2-0`, `/v3-2-6` ·
`@heroui/react@3.2.6` `package.json` + `dist/**/*.d.ts`

The rules:

* **No Provider.** Components are compound (`Card.Header`) and take React Aria props: `onPress` (not onClick), `isDisabled`, `isPending`, `isInvalid`, `isRequired`.
* Every component accepts `className`. Most accept a `render` prop that swaps the DOM element.
* Collection items need `id` + `textValue`. A Button with `slot="close"` closes its Modal, Drawer or AlertDialog.
* `package.json` has 89 subpath exports (`@heroui/react/button`, `/table`, `/toast`, …) and `sideEffects: false`. Use the subpath to lazy-load a heavy piece,
  as `@/lib/toast` does.

### 3.1 Theme switching and providers

* HeroUI's variables switch on `.dark` **or** `[data-theme="dark"]` on `<html>`, never on the media query. HeroUI's own `useTheme(default)` hook persists to
  localStorage `heroui-theme`, writes class + `data-theme`, and keeps per-instance state ("one theme controller per app").
* **Ours is `useUiStore().setTheme('system'|'light'|'dark')` + `applyTheme`.** "system" removes `data-theme`, and `theme.css` follows `prefers-color-scheme` itself,
  including HeroUI's dark-only vars (`color-scheme`, `--backdrop`, `--segment`, shadows). Calling HeroUI `useTheme` as well would create a second controller.
* `theme.css` intentionally **redefines** `@custom-variant dark`. HeroUI's version (`variants/index.css`) falls back to `prefers-color-scheme` whenever no
  dark class is present, so `dark:` utilities fire under a forced light theme. Keep exactly one definition, the one in theme.css.
* `I18nProvider` (React Aria locale for dates and numbers) and `RouterProvider` (client-side `href` navigation for every RAC link, menu item and breadcrumb) are set up
  in `@/components/AriaProviders`, which only `AppShell` renders (the public pages ship no React Aria JS). HeroUI re-exports both from `react-aria-components`.

### 3.2 Quick reference (anatomy → key props)

| component | anatomy | key props |
|---|---|---|
| Button | `<Button>` | `variant: primary\|secondary\|tertiary\|outline\|ghost\|danger` (styles also have `danger-soft`), `size: sm\|md\|lg`, `isIconOnly` (+`aria-label`), `isPending`, `fullWidth`, `type` |
| TextField | `TextField > Label, Input\|TextArea, Description, FieldError` | `value`, `onChange(string)`, `onBlur`, `isInvalid`, `isRequired`, `type`, `name`, `validationBehavior: native(default)\|aria`. `placeholder` goes on **Input** |
| NumberField | `NumberField > Label, NumberField.Group > (DecrementButton, Input, IncrementButton), FieldError` | `value: number`, `onChange(number\|undefined)`, `minValue`, `maxValue`, `step`, `formatOptions` |
| Select | `Select > Label, Select.Trigger > (Select.Value, Select.ClearButton?, Select.Indicator), Select.Popover > ListBox > ListBox.Item(id,textValue) > text + ListBox.ItemIndicator` | `value: Key\|Key[]\|null`, `onChange`, `selectionMode`, `placeholder` |
| Checkbox / Switch / Radio | **v3.2+:** `X > X.Content > (X.Control > X.Indicator\|Switch.Thumb) + plain label text`. `Description`/`FieldError` are **siblings** of `X.Content` | `isSelected`, `onChange(boolean)`. RadioGroup: `value`, `onChange(string)`, `orientation` |
| Card | `Card > Card.Header > (Card.Title h3, Card.Description p), Card.Content, Card.Footer` | `variant: transparent\|default\|secondary\|tertiary` |
| Modal | `Modal > Button, Modal.Backdrop > Modal.Container > Modal.Dialog > (Modal.CloseTrigger, Modal.Header > Modal.Heading, Modal.Body, Modal.Footer)` | Backdrop: `isOpen`, `onOpenChange`, `variant: opaque\|blur\|transparent`, `isDismissable`. Container: `size: xs…lg\|cover\|full`, `placement`, `scroll` |
| Drawer | like Modal, but `Drawer.Content` replaces the Container (+ `Drawer.Handle`) | Content: `placement: top\|bottom(default)\|left\|right` |
| AlertDialog | like Modal, with `AlertDialog.*` | Backdrop `isDismissable` defaults to **false** |
| Tabs | `Tabs > Tabs.ListContainer > Tabs.List(aria-label) > Tabs.Tab(id) > text + Tabs.Indicator`, `Tabs.Panel(id)` | `selectedKey`, `onSelectionChange(Key)`, `variant: primary\|secondary` |
| Chip / Badge | `<Chip>text</Chip>` · `Badge.Anchor > (Avatar, Badge)` | `color: default\|accent\|success\|warning\|danger`, Chip `variant: primary\|secondary\|tertiary\|soft`, Badge `placement` |
| Avatar | `Avatar > Avatar.Image, Avatar.Fallback` | `size`, `color`, `variant: default\|soft` |
| Dropdown | `Dropdown > Button, Dropdown.Popover > Dropdown.Menu > Dropdown.Item(id,textValue) > Label` | Menu `onAction(key)`, `selectionMode`. Item `variant: danger` |
| Tooltip / Popover | `Tooltip > trigger, Tooltip.Content` · `Popover > Popover.Trigger, Popover.Content > (Popover.Arrow, Popover.Dialog > Popover.Heading)` | Tooltip `delay` (700 ms), Content `placement`, `showArrow` |
| ProgressBar | `ProgressBar > Label, ProgressBar.Output, ProgressBar.Track > ProgressBar.Fill` | `value`, `isIndeterminate`, `color`, `size`. **Requires a Label or `aria-label`** |
| Skeleton / Alert | `<Skeleton className="h-4 w-32 rounded"/>` · `Alert > Alert.Indicator, Alert.Content > (Alert.Title, Alert.Description)` | Skeleton `animationType`, always **with a fixed size** (CLS). Alert `status` |
| Form / Link | `<Form onSubmit>` · `Link > text, Link.Icon?` | Form `validationBehavior`, `validationErrors`. Link is RAC Link |
| Breadcrumbs | `Breadcrumbs > Breadcrumbs.Item(href)` (the last item has no href) | `separator` |
| Pagination | `Pagination > Pagination.Summary, Pagination.Content > Pagination.Item > Previous\|Link\|Ellipsis\|Next` | presentational only. `isActive`, `isDisabled`, `onPress` |
| Accordion | `Accordion > Accordion.Item > (Accordion.Heading > Accordion.Trigger > Accordion.Indicator, Accordion.Panel > Accordion.Body)` | — |
| DatePicker | `DatePicker > Label, DateField.Group > (DateField.Input{seg => DateField.Segment}, DateField.Suffix > DatePicker.Trigger), DatePicker.Popover > Calendar…` | values come from `@internationalized/date` (`parseDate`, `today(getLocalTimeZone())`) |
| Table | `Table > Table.ScrollContainer > Table.Content > (Table.Header > Table.Column, Table.Body > Table.Row > Table.Cell)`, `Table.Footer` | `Table.Content`: `aria-label`, `sortDescriptor`, `onSortChange`. Column: `id`, `allowsSorting`, `isRowHeader`. Body: `renderEmptyState`. Use `DataTable` (§6) |
| EmptyState | `<EmptyState>` (HeroUI) | the app also has `@/components/EmptyState`, and the names clash. Import the one you mean explicitly |

### 3.3 Snippets

```tsx
import { Button, Dropdown, Label, ListBox, Modal, Select, Spinner, Tabs, useOverlayState } from '@heroui/react'
import type { Key } from '@heroui/react'
<Button isPending={m.isPending} onPress={() => m.mutate()}>                       {/* loading button: render-prop children */}
  {({ isPending }) => (<>{isPending ? <Spinner color="current" size="sm" /> : null}保存</>)}
</Button>
const [period, setPeriod] = useState<Key | null>('monthly')                       // Select (controlled)
<Select value={period} onChange={(v) => setPeriod(v as Key | null)} placeholder="选择周期">
  <Label>周期</Label><Select.Trigger><Select.Value /><Select.Indicator /></Select.Trigger>
  <Select.Popover><ListBox>
    <ListBox.Item id="monthly" textValue="月付">月付<ListBox.ItemIndicator /></ListBox.Item>
    <ListBox.Item id="yearly" textValue="年付">年付<ListBox.ItemIndicator /></ListBox.Item>
  </ListBox></Select.Popover>
</Select>
const dlg = useOverlayState()                                                      // Modal: state goes on the Backdrop
<Button onPress={dlg.open}>新建</Button>
<Modal.Backdrop isOpen={dlg.isOpen} onOpenChange={dlg.setOpen}><Modal.Container size="md"><Modal.Dialog>
  <Modal.CloseTrigger /><Modal.Header><Modal.Heading>新建节点</Modal.Heading></Modal.Header><Modal.Body>…</Modal.Body>
  <Modal.Footer><Button slot="close" variant="secondary">取消</Button><Button onPress={save}>保存</Button></Modal.Footer>
</Modal.Dialog></Modal.Container></Modal.Backdrop>
<Dropdown>                                                                         {/* trigger Button is a direct child */}
  <Button isIconOnly variant="ghost" aria-label="操作"><Ellipsis className="size-4" /></Button>
  <Dropdown.Popover placement="bottom end"><Dropdown.Menu onAction={(k) => onAction(String(k))}>
    <Dropdown.Item id="edit" textValue="编辑"><Label>编辑</Label></Dropdown.Item>
    <Dropdown.Item id="delete" textValue="删除" variant="danger"><Label>删除</Label></Dropdown.Item>
  </Dropdown.Menu></Dropdown.Popover>
</Dropdown>
<Tabs selectedKey={tab} onSelectionChange={(k) => setTab(String(k))}>
  <Tabs.ListContainer><Tabs.List aria-label="订单状态">
    <Tabs.Tab id="pending">待支付<Tabs.Indicator /></Tabs.Tab><Tabs.Tab id="paid">已完成<Tabs.Indicator /></Tabs.Tab>
  </Tabs.List></Tabs.ListContainer>
  <Tabs.Panel id="pending">…</Tabs.Panel><Tabs.Panel id="paid">…</Tabs.Panel>
</Tabs>
// toasts: import { toast } from '@/lib/toast', which wraps the HeroUI API: toast(msg, opts), toast.success / info / warning / danger,
// toast.promise(p, { loading, success, error }), toast.update(id, msg, { variant, timeout }), toast.close(id). Default timeout 4 s; timeout: 0 = persistent.
```

---

## 4. TanStack Router 1.170 (file-based)

Sources: <https://tanstack.com/router/latest/docs/framework/react/> → routing/{file-naming-conventions,routing-concepts}, guide/{authenticated-routes,
router-context,search-params,data-loading,external-data-loading,automatic-code-splitting,code-splitting,preloading,scroll-restoration,not-found-errors,
custom-link,type-safety}, api/router/{useRouterStateHook,RouteApiType,RouterType}, api/file-based-routing

### 4.1 File naming (`./src/routes` → `./src/routeTree.gen.ts`, generated by the Vite plugin)

| file | meaning |
|---|---|
| `__root.tsx` | root route (`createRootRouteWithContext`). It is **never code-split**, so keep it tiny |
| `index.tsx` · `posts.$id.tsx` = `posts/$id.tsx` | index of its parent · dynamic segment (`params.id`) |
| `_user.tsx` = `_user/route.tsx` | **pathless layout** (id `/_user`). The child `_user/dashboard.tsx` serves URL `/dashboard` |
| `x/route.tsx` = `x.route.tsx` | layout route for `/x` (`routeToken` = `route`) |
| `(group)/…` | organisational only: no URL segment, no layout |
| `-components/…`, `-columns.tsx` | the `-` prefix makes the generator ignore it (`routeFileIgnorePrefix`). Use it to colocate helpers |
| `foo_.tsx` | `_` suffix: not nested under the parent layout |
| `[.]` in a name | escapes the dot: `script[.]js.tsx` → `/script.js` |
| `posts.lazy.tsx` | legacy manual split (`createLazyFileRoute`). **Don't use it**: we have `autoCodeSplitting` |

The plugin must come **before** `react()` in `vite.config.ts`. Auto-splitting puts `component`, `errorComponent` and `notFoundComponent` in their own chunks,
and `loader` stays in the route chunk. **Never `export`** anything except `Route` from a route file, or that code lands in the main bundle. Generated route ids include
pathless segments: `createFileRoute('/_admin/admin/users')`.

### 4.2 Guards, search params, loaders, navigation

```tsx
// src/routes/_user.tsx: a pathless layout that guards every child and lazy-loads React Query
import { createFileRoute } from '@tanstack/react-router'
import { loadNamespaces } from '@/i18n'
import { requireAuth, withQueryClient } from '@/lib/guards'
export const Route = createFileRoute('/_user')({
  beforeLoad: (args) => withQueryClient(args, requireAuth),   // adds context.queryClient for every child
  loader: () => loadNamespaces(['nav', 'common']),
  component: UserLayout,                                     // <AppShell area="user" items={USER_NAV} queryClient={…} />
})

// an admin list page: the URL is the source of truth for list state
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { requireAdmin } from '@/lib/guards'
type UsersSearch = { page: number; page_size: number; sort?: string; email?: string }
export const Route = createFileRoute('/_admin/admin/users')({
  validateSearch: (s: Record<string, unknown>): UsersSearch => ({   // plain function: defaults let <Link to="/admin/users"> omit search
    page: Math.max(1, Number(s.page) || 1),
    page_size: Math.min(200, Math.max(1, Number(s.page_size) || 20)),
    ...(typeof s.sort === 'string' ? { sort: s.sort } : {}),
    ...(typeof s.email === 'string' ? { email: s.email } : {}),
  }),
  beforeLoad: requireAdmin,
  component: UsersPage,
})
function UsersPage() {
  const s = Route.useSearch()                          // typed { page, page_size, sort?, email? }
  const navigate = useNavigate({ from: Route.fullPath })
  // navigate({ search: (prev) => ({ ...prev, page: prev.page + 1 }) })
}
```

How guards behave:

* `beforeLoad` runs **parent-first**. If it throws, no child route loads.
* `throw redirect({ to: '/login', search: { redirect: location.href } })` sends the user away, and `redirect()` takes the same options as `navigate` (e.g. `replace: true`).
* Inside a `try/catch`, re-throw when `isRedirect(e)` so you don't swallow redirects.
* Return an object from `beforeLoad` to merge it into the children's `context`.

Loaders with Query: `loaderDeps` must return **only** the params the loader uses. Returning the whole `search` reloads on every param change.

```tsx
loaderDeps: ({ search: { page, status } }) => ({ page, status }),
loader: ({ context, deps }) => context.queryClient.query({ ...ordersQuery(deps), staleTime: 'static' }),  // primes the cache, = ensureQueryData
component: () => { const { data } = useSuspenseQuery(ordersQuery(Route.useLoaderDeps())); /* … */ },
```

* For paginated admin tables, **skip the loader**. Use `useQuery(adminUsersQuery(s))` (it already includes `placeholderData: keepPreviousData`) so page changes don't block navigation.
* Search params are JSON-first: `?page=2` arrives as the number `2`. In deep children, use `getRouteApi('/_admin/admin/users').useSearch()` / `.useLoaderData()`.
  Importing `Route` there creates import cycles.
* Plain loader data is read with `Route.useLoaderData()`. To 404 from data, `throw notFound()` and the nearest `notFoundComponent` renders.
* Errors with Query: in `errorComponent ({ error, reset })`, call `useQueryErrorResetBoundary().reset()` in an effect and `router.invalidate()` on retry.
* After login, call `router.history.push(safeRedirect(search.redirect))`. The docs prefer `history.push` here because it takes the full href.
* Route-level loading: `useRouterState({ select: (s) => s.isLoading })` or `s.status === 'pending'`. A `ProgressBar` used for this still needs an `aria-label`.

### 4.3 Links

* Plain navigation: `<Link to="/orders/$tradeNo" params={{ tradeNo }}>`. Styled as a button: `<Link to="/plans" className={buttonClass({ size: 'lg' })}>`.
* RAC/HeroUI components that take an `href` (Breadcrumbs.Item, Dropdown.Item, Link) navigate client-side through the `RouterProvider` in
  `AriaProviders` (`navigate`/`useHref` props per `react-aria` `.d.ts`). That provider only exists inside `AppShell`; public pages use router `Link`.
* For **typed** `to`/`params` plus intent preload on a HeroUI link, use `export const AppLink = createLink(Link /* @heroui/react */)`. TanStack documents
  `createLink` for React Aria Components v1.11+, and HeroUI's `Link` is RAC `Link`.

---

## 5. TanStack Query 5.104

Sources: `https://tanstack.com/query/latest/docs/framework/react/guides/{query-options,paginated-queries,query-invalidation,invalidations-from-mutations,
mutations,important-defaults}` · reference/classes/QueryClient · deprecations confirmed in `@tanstack/query-core@5.104.5` `.d.ts`

```tsx
import { keepPreviousData, queryOptions, useMutation, useQuery, useQueryClient, useSuspenseQuery } from '@tanstack/react-query'
import { adminApi, isApiError, type StatsRange } from '@/lib/api'
import { adminUsersQuery, meQuery, qk } from '@/lib/api/queries'
import { toast } from '@/lib/toast'

// a new factory (add it to queries.ts): always queryOptions + a qk key + forward the AbortSignal
export const adminNodeStatsQuery = (id: number, range: StatsRange = '7d') => queryOptions({
  queryKey: [...qk.admin.node(id), 'stats', range] as const,
  queryFn: ({ signal }) => adminApi.nodeStats(id, range, signal),
  staleTime: 60_000,
})
const { data: me } = useSuspenseQuery(meQuery())                  // data is never undefined
const users = useQuery(adminUsersQuery({ page: 1, page_size: 20 }))   // users.data?.items, users.data?.total
const queryClient = useQueryClient()
const ban = useMutation({
  mutationFn: (id: number) => adminApi.updateUser(id, { banned: true }),
  onSuccess: async (user) => {                                     // returning a promise keeps isPending true until the refetch is done
    queryClient.setQueryData(qk.admin.user(user.id), user)
    await queryClient.invalidateQueries({ queryKey: qk.admin.all })   // prefix match
  },
  onError: (e) => toast.danger(isApiError(e) ? e.message : String(e)),
})
await queryClient.query({ ...meQuery(), staleTime: 'static' })   // imperative read for guards and loaders
```

* **Deprecated in 5.10x:** `ensureQueryData`, `fetchQuery`, `prefetchQuery`, `ensureInfiniteQueryData`, `fetchInfiniteQuery`, `prefetchInfiniteQuery`
  ("removed in the next major"). Replace them as follows:
  * `fetchQuery(o)` → `queryClient.query(o)`
  * `ensureQueryData(o)` → `query({ ...o, staleTime: 'static' })`
  * `prefetchQuery(o)` → `query(o).catch(noop)`
  * the infinite variants → `infiniteQuery`.
  `query()` defaults to `retry: false` and ignores observer-only options (`placeholderData`, `enabled`, `refetch*`). `src/` uses none of the
  deprecated methods.
* `placeholderData: keepPreviousData` (or `(prev) => prev`) keeps the old page visible while the new key loads, and `isPlaceholderData` tells you it's stale.
* Mutation callbacks: `onMutate(variables, context)`, `onSuccess(data, variables, onMutateResult, context)`, `onError(error, variables, onMutateResult, context)`,
  `onSettled(data, error, variables, onMutateResult, context)`. `context.client` is the QueryClient.
* Defaults: data is stale immediately, inactive queries are garbage-collected after 5 min, failures retry 3× with backoff, and structural sharing is on.
  `staleTime: 'static'` means never refetch, even on `invalidateQueries`. Use `Infinity` when invalidation must still work.

---

## 6. TanStack Table v9 (9.3.1)

Sources: `https://tanstack.com/table/latest/docs/framework/react/{quick-start,guide/migrating,guide/react-compiler,guide/table-state}` ·
<https://tanstack.com/table/latest/docs/guide/client-side-vs-server-side> · HeroUI Table docs ("TanStack Table" example) · `@tanstack/table-core@9.3.1` `.d.ts`

| v8 | v9 |
|---|---|
| `useReactTable(opts)` | `useTable(opts, selector?)`. The default selector is the full state, like v8. `() => null` + `<table.Subscribe selector>` gives fine-grained renders |
| everything bundled | **required** `features: tableFeatures({ rowSortingFeature, rowPaginationFeature, … })` (or `stockFeatures`, which is large) |
| `getCoreRowModel()` | automatic |
| `getSortedRowModel()` / `getPaginationRowModel()` / `getFilteredRowModel()` | `tableFeatures` slots: `sortedRowModel: createSortedRowModel()` / `paginatedRowModel: createPaginatedRowModel()` / `filteredRowModel: createFilteredRowModel()` |
| `sortingFns`, `sortingFn`, `SortingFn` | `sortFns: { alphanumeric: sortFn_alphanumeric }`, `sortFn`, `SortFn`. Import single `sortFn_*`/`filterFn_*`; the registries are deprecated |
| `table.getState()` / `onStateChange` | `table.state` · `table.store.state` · `table.atoms.sorting.get()` / removed (use `on<Slice>Change`, `table.store.subscribe`) |
| `createColumnHelper<T>()` | `createColumnHelper<typeof features, T>()` + `helper.columns([...])` (keeps per-column `TValue`) |
| `ColumnDef<T,V>`, `Row<T>`, `Table<T>` | `ColumnDef<TFeatures,T,V>`, `Row<TFeatures,T>`, `Table<TFeatures,T>` |
| `ColumnMeta<TData,TValue>` augmentation | `ColumnMeta<TFeatures,TData,TValue>`, **or** per-table `tableFeatures({ columnMeta: metaHelper<M>() })` (`@/components/data-table` uses `columnMeta: metaHelper<DataTableColumnMeta>()`) |
| `const { getValue } = row` | **breaks**: methods moved to the prototype, so call `row.getValue('x')`. `{...row}` drops the methods |
| `flexRender(def, ctx)` | still works. Alternatively `<table.FlexRender cell={cell} />` or `<FlexRender header={h} />` |
| incremental migration | `useLegacyTable`, `getCoreRowModel`, … from `@tanstack/react-table/legacy` (deprecated, ships everything) |

`RowData` must be `Record<string, any> | any[]`. Server mode is `manualPagination: true, manualSorting: true, rowCount`. Feed controlled
`state: { pagination, sorting }` and use `functionalUpdate(updater, prev)` (exported) inside `on*Change`. Because `manualPagination` disables
`autoResetPageIndex`, reset the page yourself when sort or filters change. The HeroUI bridge maps TanStack `SortingState` to and from React Aria `SortDescriptor`
(`{ column, direction: 'ascending'|'descending' }`) through `Table.Content sortDescriptor/onSortChange`, and a header renders `Table.Column` →
`{({ sortDirection }) => <Table.SortableColumnHeader sortDirection={sortDirection}>…}`. `@/components/data-table` already does all of this.

```tsx
// e.g. src/features/admin/users/: module scope keeps a stable reference
import { Chip } from '@heroui/react'
import { createDataTableColumns } from '@/components/data-table'
import type { AdminUser } from '@/lib/api'
const col = createDataTableColumns<AdminUser>()
export const userColumns = col.columns([
  col.accessor('email', { header: '邮箱', meta: { isPrimary: true } }),
  col.accessor('banned', { header: '状态', enableSorting: false, cell: (c) =>
    <Chip size="sm" variant="soft" color={c.getValue() ? 'danger' : 'success'}>{c.getValue() ? '封禁' : '正常'}</Chip> }),
  col.accessor('balance', { header: '余额', meta: { align: 'end' }, cell: (c) => <Money cents={c.getValue()} /> }),
])
// in UsersPage (§4.2):
const q = useQuery(adminUsersQuery(s))
<DataTable aria-label="用户" columns={userColumns} data={q.data?.items} total={q.data?.total}
  page={s.page} pageSize={s.page_size} sort={s.sort} isLoading={q.isPending} isFetching={q.isFetching}
  onPageChange={(page) => navigate({ search: (prev) => ({ ...prev, page }) })}
  onSortChange={(sort) => navigate({ search: (prev) => ({ ...prev, sort, page: 1 }) })}
  getRowId={(u) => String(u.id)} rowActions={(u) => <UserActions user={u} />} />
```

Table v9 is the first version compatible with React Compiler, though nested components that only receive `row`/`cell` props need a `Subscribe` boundary. For very long client-side
tables, HeroUI offers `Virtualizer` + `TableLayout` (`rowHeight` defaults to 48).

---

## 7. TanStack Form 1.33

Sources: `https://tanstack.com/form/latest/docs/framework/react/{quick-start,guides/form-composition,guides/validation,guides/arrays,guides/basic-concepts,
guides/reactivity,guides/submission-handling}` · `@tanstack/form-core@1.33.5` `.d.ts`

API essentials:

* `createFormHookContexts()` → `{ fieldContext, formContext, useFieldContext, useFormContext }`.
* `createFormHook({ fieldContext, formContext, fieldComponents, formComponents })` → `{ useAppForm, withForm }`.
* `<form.AppField name children={(f) => <f.TextField …/>}>` gives bound field components. `<form.AppForm>` wraps form components.
* Field state lives in `field.state.value` and `field.state.meta.{errors, errorMap, isTouched, isBlurred, isDirty, isValid, isValidating}`.
  Mutate it with `field.handleChange(v)` and `field.handleBlur()`.
* Validators come in `onMount|onChange|onBlur|onSubmit` plus `…Async` forms. They can be functions (return a string or `undefined`) or Standard Schema (Zod 4).
  For async debounce, use `onChangeAsyncDebounceMs` or `asyncDebounceMs`.
* A **form-level** schema propagates its errors to the fields.
* Form state: `canSubmit`, `isSubmitting`, `isSubmitted`, `submissionAttempts`, `errorMap`. `handleSubmit()` touches every field.

Our usage, from `@/components/form` (authenticated pages; the public login/register/forgot pages use `NativeField` + `useLiteForm`
to stay inside the first-load budget, so this login form is illustrative). Its fields wire HeroUI `TextField` etc. with
`validationBehavior="aria"` and clear server errors on change:

```tsx
import { useRouter } from '@tanstack/react-router'
import { FormError, useAppForm } from '@/components/form'
import { submitError } from '@/components/form/submitError'
import { passportApi } from '@/lib/api'
import { safeRedirect } from '@/lib/guards'
import { validateEmail, validatePassword } from '@/lib/validators'
import { useAuthStore } from '@/stores/auth'

function LoginForm({ redirect }: { redirect?: string }) {
  const router = useRouter()
  const [formError, setFormError] = useState<string>()
  const form = useAppForm({
    defaultValues: { email: '', password: '' },
    onSubmit: async ({ value, formApi }) => {
      setFormError(undefined)
      try {
        const { token, user } = await passportApi.login(value)
        useAuthStore.getState().login(token, user)
        router.history.push(safeRedirect(redirect))
      } catch (e) { setFormError(submitError(formApi, e)) }   // field errors → errorMap.onServer, everything else → banner
    },
  })
  return (
    <form noValidate onSubmit={(e) => { e.preventDefault(); void form.handleSubmit() }} className="flex flex-col gap-4">
      <form.AppField name="email" validators={{ onBlur: ({ value }) => validateEmail(value) }}>
        {(f) => <f.TextField label="邮箱" type="email" autoComplete="email" />}</form.AppField>
      <form.AppField name="password" validators={{ onBlur: ({ value }) => validatePassword(value) }}>
        {(f) => <f.TextField label="密码" type="password" autoComplete="current-password" />}</form.AppField>
      <FormError message={formError} />
      <form.AppForm><form.SubmitButton fullWidth>登录</form.SubmitButton></form.AppForm>
    </form>
  )
}
```

Array field (e.g. admin route `match[]`) and async validation:

```tsx
<form.Field name="match" mode="array">
  {(arr) => (<>
    {arr.state.value.map((_, i) => <form.AppField key={i} name={`match[${i}]`}>{(f) => <f.TextField label={`规则 ${i + 1}`} />}</form.AppField>)}
    <Button variant="secondary" onPress={() => arr.pushValue('')}>添加</Button>   {/* also removeValue(i), insertValue, moveValue, swapValues, replaceValue, clearValues */}
  </>)}
</form.Field>
<form.AppField name="code" validators={{ onChangeAsyncDebounceMs: 500,
  onChangeAsync: async ({ value }) => ((await checkCode(value)) ? undefined : '邀请码无效') }}>{(f) => <f.TextField label="邀请码" />}</form.AppField>
```

* Errors from a Standard Schema are **issue objects** (`{ message }`), so `errors.join(',')` prints `[object Object]`. Use `errorText()` from `@/components/form/fieldError`.
  A form-level schema puts `Record<field, Issue[]>` in `form.state.errorMap.onChange`.
* Alternatively, server validation can live in `validators.onSubmitAsync` and return `{ form?: string, fields: { email: 'msg' } }`. In that global error shape **`fields` is required**.
* `onSubmit({ value, formApi })` receives the **input** values, not the schema-transformed ones. Call `schema.parse(value)` there if you need the transform.
* Reactive reads outside `Subscribe` use `useSelector(form.store, (s) => s.values.email)` (`useStore` is a deprecated alias). `canSubmit` stays `true` until the form is touched.
  The docs prefer `aria-disabled` over `disabled` on submit buttons.

---

## 8. TanStack Virtual 3.14

Sources: <https://tanstack.com/virtual/latest/docs/introduction> · /framework/react/react-virtual · /api/virtualizer

Installed but not used by `src/` yet (lists are paginated server-side); use this recipe when a list really needs it.

```tsx
import { type ReactNode, useRef } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
export function VirtualList<T>({ items, estimate = 44, render }: { items: readonly T[]; estimate?: number; render: (item: T, i: number) => ReactNode }) {
  const parentRef = useRef<HTMLDivElement>(null)
  const v = useVirtualizer({ count: items.length, getScrollElement: () => parentRef.current, estimateSize: () => estimate, overscan: 8,
    useFlushSync: false })   // React 19: stops "flushSync was called from inside a lifecycle method" while scrolling
  return (
    <div ref={parentRef} role="list" className="h-[480px] overflow-auto">
      <div className="relative w-full" style={{ height: v.getTotalSize() }}>
        {v.getVirtualItems().map((it) => {
          const item = items[it.index]                      // T | undefined under noUncheckedIndexedAccess
          return item === undefined ? null : (
            <div key={it.key} role="listitem" className="absolute top-0 left-0 w-full"
              style={{ height: it.size, transform: `translateY(${it.start}px)` }}>{render(item, it.index)}</div>)
        })}
      </div>
    </div>)
}
```

For variable heights, put `ref={v.measureElement}` and `data-index={it.index}` on each item. Use `useWindowVirtualizer` for page scroll. `directDomUpdates: true`
skips React re-renders during scroll, but requires `ref={v.containerRef}` on the inner element, with no height set on it.

---

## 9. zustand 5.0

Sources: zustand repo `docs/reference/middlewares/persist.md`, `docs/reference/migrations/migrating-to-v5.md`, `docs/learn/guides/beginner-typescript.md`,
`docs/reference/integrations/persisting-store-data.md` (<https://zustand.docs.pmnd.rs>)

```ts
import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import { useShallow } from 'zustand/shallow'
interface DraftState { text: string; setText: (t: string) => void }
export const useDraftStore = create<DraftState>()(            // TS: curried create<T>()(…)
  persist((set) => ({ text: '', setText: (text) => set({ text }) }),
    { name: 'gg-ticket-draft', version: 1, partialize: (s) => ({ text: s.text }) }))   // persist a subset; bump version and add migrate on shape changes
const user = useAuthStore((s) => s.user)                                            // single value: fine
const { theme, setTheme } = useUiStore(useShallow((s) => ({ theme: s.theme, setTheme: s.setTheme })))   // new object each time → useShallow
useAuthStore.getState().token                                                       // outside React (guards, API client)
```

* v5 `create` takes no equality function. A selector that returns a **new** object or array each call causes "Maximum update depth exceeded".
  Fix it with `useShallow` (or `createWithEqualityFn` from `zustand/traditional`). A fallback like `s.fn ?? (() => {})` is also unstable, so hoist the constant.
* A synchronous storage (`localStorage`) means the store is **already hydrated at creation**. Async storages hydrate in a microtask instead. `useAuthStore` additionally exposes a `hydrated` flag.
  Persist API: `useX.persist.hasHydrated()`, `onFinishHydration(cb)`, `rehydrate()`, `clearStorage()`.
* `persist` **no longer writes the initial state** at creation (v5 / v4.5.5). The default storage is unvalidated JSON (`createJSONStorage(() => localStorage)`).

---

## 10. motion 14

Sources: <https://motion.dev/docs/react-upgrade-guide> · /react-lazy-motion · /react-reduce-bundle-size · /react-use-reduced-motion · /react-animate-presence ·
/react-accessibility

Import from **`motion/react`**, never `framer-motion`. Versions 12 through 14 have no React-facing breaking changes. v13 removed the optional `@emotion/is-prop-valid`.
Sizes: `motion.*` is ~43 kB and can't be tree-shaken. `m` + `LazyMotion` is ~5 kB initially, `domAnimation` adds +24 kB (load it async), `domMax` +38 kB (drag/layout), and `useReducedMotion` ~1 kB.

```tsx
// motion-features.ts
import { domAnimation } from 'motion/react'
export default domAnimation
// wrapper around the animated area (lazy route, not the root)
import { LazyMotion, MotionConfig } from 'motion/react'
const loadFeatures = () => import('./motion-features').then((res) => res.default)
<MotionConfig reducedMotion="user"><LazyMotion features={loadFeatures}>{children}</LazyMotion></MotionConfig>
// animate
import * as m from 'motion/react-m'
import { AnimatePresence } from 'motion/react'
<AnimatePresence initial={false} mode="wait">   {/* mode: sync (default) | wait | popLayout; direct children need unique keys */}
  {open && <m.div key="panel" initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: 8 }} transition={{ duration: 0.24, ease: 'easeOut' }} />}
</AnimatePresence>
```

`reducedMotion="user"` disables transform and layout animations but keeps opacity and color. `useReducedMotion()` (from motion, or from `@/lib/motion`) returns a boolean.
**Don't** use `<LazyMotion strict>` where `motion.*` components render (strict mode throws); the effects in `@/components/effects` use plain CSS/rAF and need no `motion`.
HeroUI animates with CSS and honours its own `motion-reduce:` variant (and `data-reduce-motion`).

---

## 11. Effects (`web/src/components/effects`)

React Bits (reactbits.dev) is **MIT + Commons Clause**, which is not GPL-compatible, so its source is NOT used. The
decorative effects are original: `ShinyText` and `GradientText` (CSS `background-clip: text` keyframes), `BlurText`
(IntersectionObserver + staggered CSS reveal), `SpotlightCard` (pointer-driven CSS variables + radial gradient) and
`CountUp` (requestAnimationFrame with ease-out-expo, honours reduced motion). Always render them through `WhenIdle`
or `lazy` so they never enter a route's first chunk; `effects.css` disables every animation under
`prefers-reduced-motion: reduce`.

## 12. lucide-react v1

Sources: lucide repo `docs/guide/react/{migration,advanced/accessibility,advanced/dynamic-icon-component,advanced/aliased-names}.md`

* Import icons by name (`import { House } from 'lucide-react'`) so they tree-shake. **Never** use `DynamicIcon` (`lucide-react/dynamic`): it pulls every icon into the build.
* v1 **removed the brand icons** Github, Gitlab, Slack, Figma, Facebook, Instagram, LinkedIn, Dribbble, Codepen, Codesandbox, Framer, Pocket, Chromium and RailSymbol.
  Use the official SVGs or Simple Icons instead.
* Icons ship with `aria-hidden="true"`. An icon that carries meaning needs an `aria-label` or a `<title>` child. An icon-only **Button** needs its own `aria-label`.
  Size icons with `className="size-4"`; color comes from `currentColor`. Aliases exist (`House` = `HouseIcon` = `LucideHouse`).

---

## 13. Vite 8 build

Sources: <https://vite.dev/guide/migration> · <https://vite.dev/config/build-options> · <https://rolldown.rs/in-depth/manual-code-splitting> ·
<https://rolldown.rs/reference/OutputOptions.codeSplitting> · <https://github.com/nonzzz/vite-plugin-compression> (README)

* Vite 8 bundles with **Rolldown** and transforms and minifies JS with **Oxc**. CSS is minified by Lightning CSS. esbuild is now only an optional dependency:
  `build.minify: 'esbuild'` and `transformWithEsbuild` are deprecated, and `esbuild.*` options no longer apply.
* `build.rollupOptions` is now **`build.rolldownOptions`** (the old name is a deprecated alias). `output.manualChunks` changed too: the object form is **removed** and the function form is
  deprecated. Use `output.codeSplitting: { groups: [{ name, test: /regex/, priority?, minSize?, maxSize?, minShareCount? }] }`.
  A group becomes one chunk, so **only group libraries every route needs**. The `heroui` group sets `entriesAware: true` for that reason.
* `build.target` defaults to `'baseline-widely-available'` (chrome111/edge111/firefox114/safari16.4/ios16.4); `vite.config.ts` keeps the default
  (the same floor as Tailwind v4) and sets `cssMinify: 'lightningcss'`, `modulePreload.polyfill: false`, `chunkSizeWarningLimit: 400`.
  `assetsInlineLimit` defaults to 4096 bytes, and `chunkSizeWarningLimit` compares against **uncompressed** size.
* Compression: `compression({ algorithms: ['brotliCompress', 'gzip'], include, threshold })`. For tuned levels use `defineAlgorithm('gzip', { level: 9 })` and
  `defineAlgorithm('brotliCompress', { params: { [constants.BROTLI_PARAM_QUALITY]: 11 } })` with `import { constants } from 'node:zlib'`. The README's
  `require('zlib')` doesn't work in an ESM config. Originals are kept (`deleteOriginalAssets: false`), and the Go server picks `.br`/`.gz`.
* Plugin order: `tanstackRouter()` → `react()` → `herouiComponentCss()` (`scripts/heroui-css.ts`, per-component HeroUI CSS) → `tailwindcss()` →
  `compression()` → `productionPreview()` (`scripts/preview.ts`). Budget: the first-load gzip size (shown in the build report) stays under 120 kB.
* Dev proxy: `/api` and `/s/` → `GGBOARD_API` (default `http://localhost:8080`); `vite preview` proxies `/api`.

---

## 14. Lighthouse 100 (`@lhci/cli` 0.15)

Sources: <https://github.com/GoogleChrome/lighthouse-ci/blob/main/docs/configuration.md> · Lighthouse `core/config/default-config.js` (scored audits) ·
`core/lib/network-request.js` (localhost counts as secure) · `core/audits/seo/robots-txt.js`

`pnpm -C web lh` builds and runs `lhci autorun --config=./lighthouserc.cjs` (`.cjs` because the package is `"type": "module"`). The config:

* serves `dist/` with `vite preview --mode lighthouse --port 4173` (`scripts/preview.ts`: precompressed `.br`/`.gz`, immutable hashed assets,
  `no-cache` `index.html`, fixture answers for `/api/v1/guest/*`, the passport endpoints and `/user/me`);
* audits `/`, `/login` and `/register`, 3 runs, desktop preset (`LH_PRESET=mobile` → Lighthouse's mobile emulation; `pnpm lh:mobile`);
  `CHROME_PATH` overrides the browser;
* asserts **all four categories at `minScore: 1`** (performance on the `median-run`);
* writes reports to `web/.lighthouseci/reports` (filesystem target, no upload).
Presets: `lighthouse:recommended` and `lighthouse:no-pwa` warn on metrics. Assertions take `minScore`, `maxNumericValue`, `maxLength`, and
`aggregationMethod: median|optimistic|pessimistic|median-run`.

Authenticated URLs render the login redirect, so they audit the shell. Checklist of scored audit IDs:

* **Performance**: FCP, LCP, TBT, CLS, SI. Split routes, use vendor groups, load no web fonts, `lazy()` heavy widgets, give Skeletons fixed sizes, put `width`/`height` on `<img>`,
  start animations after idle, keep `__root` small, and lazy-load toasts (already done).
* **Accessibility**:
  * `html-has-lang`: `syncDocumentLanguage()` keeps it in sync.
  * `document-title`: React 19 hoists `<title>` from anywhere, but its children must be **one string**, e.g. `<title>{`${t} · GG Bond`}</title>`.
  * Structure: `landmark-one-main` (exactly one `<main>`), `heading-order`, `skip-link`, `tabindex`, `target-size`.
  * Names: `button-name` (icon-only Buttons), `label`/`select-name` (always render `<Label>`), `aria-progressbar-name`, `aria-dialog-name` (render `Modal.Heading`), `link-name`.
  * `color-contrast` (§2).
* **Best practices**: `errors-in-console` (zero errors in prod, no 404s for icons or manifest), `deprecations`, `inspector-issues`, `doctype`, `charset`, `image-aspect-ratio`,
  `image-size-responsive`, `paste-preventing-inputs` (don't block paste in password fields). `is-on-https` passes on localhost.
* **SEO**: `meta-description`, `document-title`, `is-crawlable` (no `noindex`), and `robots-txt`: `public/robots.txt` exists, and note that a 4xx makes the audit
  not-applicable while a 5xx fails it. Also `crawlable-anchors` and `link-text` (use a real `<a href>` via router `Link`, never `onClick` navigation, with descriptive text), plus `http-status-code`, `canonical` and `hreflang`.
* Server (AGENTS §6): serve `.br`/`.gz` with `Content-Encoding`, hashed assets with `Cache-Control: public, max-age=31536000, immutable`, and `index.html` with `no-cache`,
  and send a CSP.

---

## 15. Gotchas (verified; read them all)

1. **HeroUI v3 ≠ v2.**
   * There is no `HeroUIProvider`, and Button has no `color` prop (it has `variant`).
   * Use `isPending`, not `isLoading`, and `onPress`, not `onClick`.
   * Parts are compound (`Card.Header`, `Modal.Backdrop`), and `Text` was renamed `Typography` in v3.0.5.
2. **HeroUI peers are mandatory** (`react-aria`, `react-aria-components`, `@react-aria/ssr`, `@react-aria/utils`, `@internationalized/date`, plus `react>=19` and `tailwindcss>=4`).
   CSS order is `@import "tailwindcss"` and **then** `@import "@heroui/styles"`.
3. **v3.2 breaking change** for Checkbox, Radio and Switch:
   * `X.Control` now sits **inside** `X.Content`.
   * The label is plain text, with no nested `<Label>`.
   * `Description`/`FieldError` are siblings of `X.Content`.
   The Switch page's *Anatomy* block still shows the v3.1 shape, so trust the examples and the release notes.
4. **Dark mode**: HeroUI's variables need `.dark` or `[data-theme=dark]`. Its built-in `dark:` variant also fires through `prefers-color-scheme` under a forced light
   theme, which is why `theme.css` overrides it. Don't use HeroUI `useTheme` next to `useUiStore`: that makes two controllers and two storage keys.
5. Modal, Drawer and AlertDialog are controlled through `*.Backdrop` (`isOpen`/`onOpenChange`). AlertDialog is not backdrop-dismissable by default.
6. Select takes `value`/`onChange` with `Key | Key[] | null`, and items need `id` + `textValue`. `placeholder` goes on `<Input>`, not on `<TextField>`.
   TextField and Form default to `validationBehavior="native"` (browser bubbles, blocked submit). Use `"aria"` with TanStack Form.
7. `RouterProvider` exists in both `@tanstack/react-router` and `@heroui/react`/`react-aria-components`. Alias one of them. The same goes for `EmptyState` (HeroUI vs `@/components/EmptyState`).
8. **Query 5.10x**: `ensureQueryData`, `fetchQuery`, `prefetchQuery` (and the infinite variants) are deprecated in favor of `queryClient.query()`/`infiniteQuery()`.
   The 3rd mutation-callback argument is `onMutateResult`.
9. **Router**:
   * The plugin must run before `react()`.
   * Never export route properties; it silently breaks code splitting.
   * `loaderDeps` must be minimal.
   * Use `defaultPreloadStaleTime: 0` with Query.
   * `validateSearch` is a plain function (no schema library is installed).
   * `scrollRestoration: true` replaces the deprecated `<ScrollRestoration/>`.
   * `routeTree.gen.ts` (git-ignored, lint-ignored) must exist before `tsc`: `pnpm typecheck|lint|test` run `scripts/gen-routes.mjs` first.
10. **Table v9**:
    * Use `useTable` with the required `features`.
    * `sortingFn*` is now `sortFn*`.
    * Read state via `table.state`, not `getState()`.
    * Generics now start with `TFeatures`.
    * Don't destructure row or column methods.
    * Import individual `sortFn_*`/`filterFn_*`; the registries are deprecated.
    * The legacy API lives in `@tanstack/react-table/legacy`.
11. **Form**: Standard Schema errors are objects, so use `errorText()`. The global validator error requires `fields`. `useStore` is now `useSelector`. `onSubmit` receives the input values.
12. **Virtual + React 19**: set `useFlushSync: false`. **zustand v5**: selectors must return stable references (`useShallow`), and `persist` no longer writes the initial state.
13. **Vite 8**: `rolldownOptions` plus `output.codeSplitting.groups`; `rollupOptions`/`manualChunks` are deprecated or removed, and there is no esbuild by default.
14. **TypeScript 7**:
    * There is no compiler API, so typescript-eslint gets TypeScript 6.0.3 through `web/.pnpmfile.cjs` (§1).
    * `types` defaults to `[]`, and `baseUrl` is removed.
    * `noUncheckedIndexedAccess` is on in `tsconfig.app.json`, so `arr[i]` may be `undefined`.
    * `erasableSyntaxOnly` rules out `enum` and parameter properties.
    * The router plugin and generator depend on Babel and Prettier, not `typescript`, and no TS 7 incompatibility is documented.
15. **Apple colors vs AA**: raw system colors with white text, 60% secondary label text, and `systemGray` text all fail `color-contrast`. Use the `label-*`/`tint` tokens.
    `--tint` sits at 4.58:1, so any tweak must be re-measured.
16. **Effects** are original (`@/components/effects`, see §11); never copy React Bits source (MIT + Commons Clause, not GPL-compatible).
17. **lucide v1** has no brand icons, and `DynamicIcon` defeats tree-shaking.
18. A `ProgressBar` or `Meter` without a Label or `aria-label` fails `aria-progressbar-name`, and an icon-only `Button` without `aria-label` fails `button-name`.
    React 19 `<title>` needs a single string child: write `{`订单 ${n}`}`, not `订单 {n}`.
