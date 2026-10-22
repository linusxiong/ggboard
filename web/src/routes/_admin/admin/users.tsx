import { createFileRoute, type SearchSchemaInput } from '@tanstack/react-router'
import { parseUserSearch, type UserSearch } from '@/features/admin/users/filters'
import { UsersPage } from '@/features/admin/users/UsersPage'
import { loadNamespaces } from '@/i18n'

/**
 * `/admin/users` — staff may list and view users (the `_admin` layout already requires staff);
 * write actions are shown to admins only and enforced by the backend.
 */
export const Route = createFileRoute('/_admin/admin/users')({
  validateSearch: (search: Partial<UserSearch> & SearchSchemaInput) => parseUserSearch(search),
  loader: () => loadNamespaces(['admin-users', 'common']),
  component: UsersPage,
})
