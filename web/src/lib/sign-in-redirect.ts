/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

/**
 * Loop guard for the full-page sign-in redirect after a rejected session.
 *
 * Each redirect reloads the app, which fires every page query again. If the
 * browser keeps a session the server keeps rejecting, the reloads repeat about
 * once a second and exhaust the per-IP API rate limit for everyone behind the
 * same address. Redirects are therefore counted per tab, and after a few in
 * quick succession the tab stays where it is.
 */
export const SIGN_IN_REDIRECTS_KEY = 'new-api:sign-in-redirects'
export const SIGN_IN_REDIRECT_WINDOW_MS = 60_000
export const MAX_SIGN_IN_REDIRECTS = 3

/**
 * Record a sign-in redirect and report whether it may proceed.
 *
 * Unavailable or corrupted storage allows the redirect: the guard only limits
 * loops and must never keep a signed-out user away from the sign-in page.
 */
export function claimSignInRedirect(
  storage: Storage | undefined,
  now = Date.now()
): boolean {
  let recent: number[] = []
  try {
    const stored: unknown = JSON.parse(
      storage?.getItem(SIGN_IN_REDIRECTS_KEY) ?? '[]'
    )
    if (Array.isArray(stored)) {
      recent = stored.filter(
        (time): time is number =>
          typeof time === 'number' &&
          now - time >= 0 &&
          now - time < SIGN_IN_REDIRECT_WINDOW_MS
      )
    }
  } catch {
    recent = []
  }

  if (recent.length >= MAX_SIGN_IN_REDIRECTS) return false

  try {
    storage?.setItem(SIGN_IN_REDIRECTS_KEY, JSON.stringify([...recent, now]))
  } catch {
    // Without storage the guard cannot count; let the redirect through.
  }
  return true
}
