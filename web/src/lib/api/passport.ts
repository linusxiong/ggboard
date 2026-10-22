/** Auth endpoints (`/passport`, 🔓, rate limited) — docs/API.md §Passport. */
import { api } from './client'
import type { AuthResult, ForgotInput, LoginInput, RegisterInput, SendCodeInput } from './types'

/** `POST /passport/register` → `{token, user}`. 403 when `stop_register`. */
export const register = (input: RegisterInput) =>
  api.post<AuthResult>('/passport/register', input, { anonymous: true })

/** `POST /passport/login` → `{token, user}`. */
export const login = (input: LoginInput) =>
  api.post<AuthResult>('/passport/login', input, { anonymous: true })

/** `POST /passport/forgot` → `true`. */
export const forgot = (input: ForgotInput) => api.post<true>('/passport/forgot', input, { anonymous: true })

/** `POST /passport/email/send-code` → `true` (60s cooldown per email, 10/h per IP). */
export const sendEmailCode = (input: SendCodeInput) =>
  api.post<true>('/passport/email/send-code', input, { anonymous: true })

/** `POST /passport/login/token` — one-time login token from an admin "login as" URL. */
export const loginWithToken = (token: string) =>
  api.post<AuthResult>('/passport/login/token', { token }, { anonymous: true })

/** Cooldown the backend enforces between two `send-code` calls for the same email (seconds). */
export const EMAIL_CODE_COOLDOWN = 60
