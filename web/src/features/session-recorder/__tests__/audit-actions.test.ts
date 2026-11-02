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
import { createInstance } from 'i18next'
import { describe, expect, test } from 'vitest'

import { renderAuditContent } from '@/features/usage-logs/lib/format'
import type { LogOtherData } from '@/features/usage-logs/types'
import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'

async function translator(lng: 'en' | 'zh') {
  const i18n = createInstance()
  await i18n.init({
    lng,
    resources: { en, zh },
    interpolation: { escapeValue: false },
  })
  return i18n.t.bind(i18n) as (
    key: string,
    opts?: Record<string, unknown>
  ) => string
}

function op(action: string, params?: Record<string, unknown>) {
  return { op: { action, params } } as LogOtherData
}

describe('session recorder audit actions', () => {
  test('a content view names the record and session it opened', async () => {
    const t = await translator('en')
    expect(
      renderAuditContent(
        op('session_recorder.view_content', {
          record_id: 'rec-1',
          session_id: 'sess-1',
        }),
        t
      )
    ).toBe('Viewed recorded content of record rec-1 (session sess-1)')
  })

  test.each([
    'session_recorder.revenue_rules_update',
    'session_recorder.revenue_rules_test',
    'session_recorder.backfill_start',
    'session_recorder.backfill_cancel',
  ])('%s renders a translated description', async (action) => {
    const zhT = await translator('zh')
    const enT = await translator('en')
    const english = renderAuditContent(op(action), enT)
    expect(english).toBeTruthy()
    expect(renderAuditContent(op(action), zhT)).not.toBe(english)
  })
})
