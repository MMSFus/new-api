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
import { describe, expect, it } from 'vitest'

import { getTieredBillingSummary } from '../lib/format'
import type { LogOtherData } from '../types'

const b64 = (text: string) => Buffer.from(text, 'utf8').toString('base64')

function tieredLog(overrides: Partial<LogOtherData>): LogOtherData {
  return {
    billing_mode: 'tiered_expr',
    expr_b64: b64('tier("base", p * 8 + c * 16)'),
    matched_tier: 'base',
    ...overrides,
  } as LogOtherData
}

const prices = (other: LogOtherData) =>
  Object.fromEntries(
    getTieredBillingSummary(other)?.priceEntries.map((entry) => [
      entry.field,
      entry.price,
    ]) ?? []
  )

describe('consume log prices written in another currency', () => {
  it('converts raw expression coefficients with the frozen factor', () => {
    expect(
      prices(tieredLog({ price_currency: 'CNY', currency_usd_factor: 0.125 }))
    ).toMatchObject({ inputPrice: 1, outputPrice: 2 })
  })

  it('converts a raw fixed price with the frozen factor', () => {
    const other = tieredLog({
      expr_b64: b64('tier("call", fixed(0.8))'),
      matched_tier: 'call',
      billing_unit: 'request',
      fixed_price: 0.8,
      price_currency: 'CNY',
      currency_usd_factor: 0.125,
    })
    expect(prices(other).fixedPrice).toBeCloseTo(0.1)
  })

  it('leaves USD logs without currency fields unchanged', () => {
    expect(prices(tieredLog({}))).toMatchObject({
      inputPrice: 8,
      outputPrice: 16,
    })
  })
})
