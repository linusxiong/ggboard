/** Auth endpoints (`/passport`, 🔓, rate limited) — docs/API.md §Passport. */
import { api } from './client'
import type {
  AuthResult,
  ForgotInput,
  LoginInput,
  LoginResult,
  MfaChallenge,
  PasskeyAssertion,
  PasskeyLoginBegin,
  RegisterInput,
  SendCodeInput,
} from './types'

/** `POST /passport/register` → `{token, user}`. 403 when `stop_register`. */
export const register = (input: RegisterInput) =>
  api.post<AuthResult>('/passport/register', input, { anonymous: true })

/**
 * `POST /passport/login` → `{token, user}`, or `{mfa_required, mfa_token, methods, has_passkey}` when the
 * account has 2FA (narrow with `isMfaChallenge`). 401 wrong credentials, 403 banned, 429 locked (`Retry-After`).
 */
export const login = (input: LoginInput) =>
  api.post<LoginResult>('/passport/login', input, { anonymous: true })

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

// ---- FE-1 ---- second factor and passkey sign-in

export const isMfaChallenge = (result: LoginResult): result is MfaChallenge =>
  (result as Partial<MfaChallenge>).mfa_required === true

/**
 * `POST /passport/login/mfa` — `code` is a 6-digit TOTP or a backup code `xxxxx-xxxxx`.
 * 401 `mfa_invalid` (wrong code), 401 `mfa_expired` (start over), 429 after 5 tries.
 */
export const loginMfa = (input: { mfa_token: string; code: string }) =>
  api.post<AuthResult>('/passport/login/mfa', input, { anonymous: true })

/** `POST /passport/passkey/login/begin` — omit `email` for usernameless (discoverable) sign-in. */
export const passkeyLoginBegin = (email?: string) =>
  api.post<PasskeyLoginBegin>('/passport/passkey/login/begin', email ? { email } : {}, { anonymous: true })

/** `POST /passport/passkey/login/finish` → `{token, user}`. */
export const passkeyLoginFinish = (input: { challenge_id: string; credential: PasskeyAssertion }) =>
  api.post<AuthResult>('/passport/passkey/login/finish', input, { anonymous: true })
