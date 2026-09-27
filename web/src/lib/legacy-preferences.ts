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
import { removeCookie } from '@/lib/cookies'
import { THEME_STORAGE_KEYS, writeThemePreference } from '@/lib/theme-storage'

// Font, radius, scale, layout and direction lost their settings when the
// appearance panel was trimmed to color theme and light/dark. Clear any
// saved values so visitors aren't stuck with choices they can't undo.
const LEGACY_STORAGE_KEYS = [
  THEME_STORAGE_KEYS.font,
  THEME_STORAGE_KEYS.radius,
  THEME_STORAGE_KEYS.scale,
  THEME_STORAGE_KEYS.contentLayout,
]
const LEGACY_COOKIES = ['font', 'dir', 'layout_variant', 'layout_collapsible']

export function clearLegacyAppearancePreferences(): void {
  for (const key of LEGACY_STORAGE_KEYS) writeThemePreference(key, null)
  for (const name of LEGACY_COOKIES) removeCookie(name)
}
