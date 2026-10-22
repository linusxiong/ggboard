/**
 * `pnpm exec vite --config scripts/mock/vite.config.ts` — the regular dev server plus the in-memory
 * user-portal mock API (scripts/mock/user-portal.ts). Dev only.
 */
import { mergeConfig } from 'vite'
import base from '../../vite.config.ts'
import { userPortalMock } from './user-portal.ts'

export default mergeConfig(base, { plugins: [userPortalMock()] })
