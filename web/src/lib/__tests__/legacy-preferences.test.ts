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
import { beforeEach, expect, test } from 'vitest'

import { getCookie, setCookie } from '@/lib/cookies'
import { clearLegacyAppearancePreferences } from '@/lib/legacy-preferences'
import { THEME_STORAGE_KEYS } from '@/lib/theme-storage'

beforeEach(() => {
  window.localStorage.clear()
})

test('drops appearance preferences that no longer have a setting', () => {
  window.localStorage.setItem(THEME_STORAGE_KEYS.font, 'serif')
  window.localStorage.setItem(THEME_STORAGE_KEYS.radius, 'xl')
  window.localStorage.setItem(THEME_STORAGE_KEYS.scale, 'lg')
  window.localStorage.setItem(THEME_STORAGE_KEYS.contentLayout, 'centered')
  setCookie('font', 'serif')
  setCookie('dir', 'rtl')
  setCookie('layout_variant', 'floating')
  setCookie('layout_collapsible', 'offcanvas')

  clearLegacyAppearancePreferences()

  expect(window.localStorage.getItem(THEME_STORAGE_KEYS.font)).toBeNull()
  expect(window.localStorage.getItem(THEME_STORAGE_KEYS.radius)).toBeNull()
  expect(window.localStorage.getItem(THEME_STORAGE_KEYS.scale)).toBeNull()
  expect(
    window.localStorage.getItem(THEME_STORAGE_KEYS.contentLayout)
  ).toBeNull()
  for (const name of ['font', 'dir', 'layout_variant', 'layout_collapsible']) {
    expect(getCookie(name)).toBeFalsy()
  }
})

test('keeps the settings that are still offered', () => {
  window.localStorage.setItem(THEME_STORAGE_KEYS.mode, 'dark')
  window.localStorage.setItem(THEME_STORAGE_KEYS.preset, 'teal')
  setCookie('sidebar_state', 'false')

  clearLegacyAppearancePreferences()

  expect(window.localStorage.getItem(THEME_STORAGE_KEYS.mode)).toBe('dark')
  expect(window.localStorage.getItem(THEME_STORAGE_KEYS.preset)).toBe('teal')
  expect(getCookie('sidebar_state')).toBe('false')
})
