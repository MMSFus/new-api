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
import { afterEach, expect, test } from 'vitest'

import {
  claimSignInRedirect,
  MAX_SIGN_IN_REDIRECTS,
  SIGN_IN_REDIRECT_WINDOW_MS,
  SIGN_IN_REDIRECTS_KEY,
} from '@/lib/sign-in-redirect'

afterEach(() => {
  window.sessionStorage.clear()
})

test('redirects stop once a tab has redirected repeatedly within the window', () => {
  const start = 1_000_000
  const allowed = Array.from({ length: MAX_SIGN_IN_REDIRECTS + 2 }, (_, i) =>
    claimSignInRedirect(window.sessionStorage, start + i * 1000)
  )

  expect(allowed).toEqual([
    ...Array.from({ length: MAX_SIGN_IN_REDIRECTS }, () => true),
    false,
    false,
  ])
})

test('redirects are allowed again after the window has passed', () => {
  const start = 1_000_000
  for (let i = 0; i < MAX_SIGN_IN_REDIRECTS; i++) {
    claimSignInRedirect(window.sessionStorage, start + i)
  }

  expect(
    claimSignInRedirect(
      window.sessionStorage,
      start + SIGN_IN_REDIRECT_WINDOW_MS + MAX_SIGN_IN_REDIRECTS
    )
  ).toBe(true)
})

test('corrupted or unavailable storage never blocks the redirect', () => {
  window.sessionStorage.setItem(SIGN_IN_REDIRECTS_KEY, '{not json')
  expect(claimSignInRedirect(window.sessionStorage)).toBe(true)

  const throwing = {
    getItem: () => {
      throw new Error('blocked')
    },
    setItem: () => {
      throw new Error('blocked')
    },
  } as unknown as Storage
  expect(claimSignInRedirect(throwing)).toBe(true)
  expect(claimSignInRedirect(undefined)).toBe(true)
})
