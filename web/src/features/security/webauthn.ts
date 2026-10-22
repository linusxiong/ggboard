/**
 * Passkey (WebAuthn) helpers. `@simplewebauthn/browser` is loaded on demand so neither the login page
 * nor the profile page pays for it up front (AGENTS.md §6).
 *
 *   if (supportsWebAuthn()) {
 *     const { startAuthentication } = await loadWebAuthn()
 *     try { … } catch (e) { if (webAuthnFailure(e) === 'cancelled') return }
 *   }
 */
import type * as WebAuthnModule from '@simplewebauthn/browser'

type WebAuthnLib = typeof WebAuthnModule

let lib: Promise<WebAuthnLib> | null = null

export function loadWebAuthn(): Promise<WebAuthnLib> {
  lib ??= import('@simplewebauthn/browser').catch((error: unknown) => {
    lib = null
    throw error
  })
  return lib
}

/** Same check as `browserSupportsWebAuthn()`, without loading the library. */
export function supportsWebAuthn(): boolean {
  return typeof window !== 'undefined' && typeof window.PublicKeyCredential === 'function'
}

/**
 * Conditional UI ("passkey autofill" in the username field). Uses the native check first so browsers
 * without it never download the library.
 */
export async function supportsWebAuthnAutofill(): Promise<boolean> {
  if (!supportsWebAuthn()) return false
  const native = (
    window.PublicKeyCredential as unknown as { isConditionalMediationAvailable?: () => Promise<boolean> }
  ).isConditionalMediationAvailable
  if (typeof native !== 'function') return false
  try {
    return await native.call(window.PublicKeyCredential)
  } catch {
    return false
  }
}

/**
 * - `cancelled`: the user (or their password manager) dismissed the prompt, it timed out, or another
 *   ceremony replaced it (`NotAllowedError` / `AbortError`) — show nothing alarming;
 * - `duplicate`: this authenticator already holds a passkey for the account (`InvalidStateError`);
 * - `unsupported`: no WebAuthn in this browser / context;
 * - `security`: wrong origin / RP ID (deployment problem);
 * - `error`: anything else.
 */
export type WebAuthnFailure = 'cancelled' | 'duplicate' | 'unsupported' | 'security' | 'error'

export function webAuthnFailure(error: unknown): WebAuthnFailure {
  const name = typeof error === 'object' && error !== null && 'name' in error ? String(error.name) : ''
  const code = typeof error === 'object' && error !== null && 'code' in error ? String(error.code) : ''
  const message = error instanceof Error ? error.message : ''
  if (name === 'NotAllowedError' || name === 'AbortError' || code === 'ERROR_CEREMONY_ABORTED')
    return 'cancelled'
  if (name === 'InvalidStateError' || code === 'ERROR_AUTHENTICATOR_PREVIOUSLY_REGISTERED') return 'duplicate'
  if (name === 'NotSupportedError' || /not supported/i.test(message)) return 'unsupported'
  if (name === 'SecurityError' || code === 'ERROR_INVALID_DOMAIN' || code === 'ERROR_INVALID_RP_ID')
    return 'security'
  return 'error'
}

/** A friendly default name for a new passkey ("Mac", "iPhone", "Windows" …). */
export function guessDeviceName(
  userAgent = typeof navigator === 'undefined' ? '' : navigator.userAgent,
): string {
  const ua = userAgent.toLowerCase()
  if (ua.includes('iphone')) return 'iPhone'
  if (ua.includes('ipad')) return 'iPad'
  if (ua.includes('android')) return 'Android'
  if (ua.includes('mac os') || ua.includes('macintosh')) return 'Mac'
  if (ua.includes('windows')) return 'Windows'
  if (ua.includes('cros')) return 'Chromebook'
  if (ua.includes('linux')) return 'Linux'
  return 'Passkey'
}
