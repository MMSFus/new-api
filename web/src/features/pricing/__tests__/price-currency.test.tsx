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
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, test } from 'vitest'

import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { DynamicPricingBreakdown } from '../components/dynamic-pricing-breakdown'
import {
  getCardExamplePrice,
  getDynamicPricingSummary,
} from '../lib/dynamic-price'
import type { PricingModel } from '../types'

// GET /api/pricing for a CNY-marked model at 1 USD = 8 CNY: the expression
// text keeps CNY coefficients; currency_usd_factor converts them to USD.
function cnyModel(overrides: Partial<PricingModel> = {}): PricingModel {
  return {
    id: 1,
    model_name: 'cny-model',
    quota_type: 0,
    model_ratio: 0,
    completion_ratio: 1,
    enable_groups: ['default'],
    billing_mode: 'tiered_expr',
    billing_expr: 'tier("base", p * 8 + c * 16)',
    price_currency: 'CNY',
    currency_usd_factor: 0.125,
    ...overrides,
  }
}

function entryValue(model: PricingModel, field: string) {
  return getDynamicPricingSummary(model, { tokenUnit: 'M' })?.entries.find(
    (entry) => entry.field === field
  )
}

afterEach(() => {
  cleanup()
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
})

describe('expression prices written in another currency', () => {
  test('multiplies expression coefficients by currency_usd_factor for USD display', () => {
    expect(entryValue(cnyModel(), 'inputPrice')).toMatchObject({
      value: 1,
      formatted: '$1',
    })
    expect(entryValue(cnyModel(), 'outputPrice')?.value).toBe(2)
  })

  test('shows the raw coefficient when the factor is missing because the rate is invalid', () => {
    const model = cnyModel({ currency_usd_factor: undefined })
    expect(entryValue(model, 'inputPrice')?.value).toBe(8)
  })

  test('scales a fixed per-request price in the expression', () => {
    const model = cnyModel({ billing_expr: 'tier("call", fixed(0.8))' })
    expect(entryValue(model, 'fixedPrice')?.value).toBeCloseTo(0.1)
  })

  test('scales the task example price of a task model', () => {
    const model = cnyModel({
      billing_expr: 'tier("base", u("tokens") * 8 / 1000000)',
      billing_usage_schema: { tokens: { type: 'number', unit: 'token' } },
      billing_usage_examples: [{ label: '720p', facts: { tokens: 500000 } }],
    })
    // 500k tokens × 8 CNY / 1M = 4 CNY = 0.5 USD.
    expect(getCardExamplePrice(model, { tokenUnit: 'M' })?.total).toBeCloseTo(
      0.5
    )
  })

  test('the breakdown table renders converted USD unit prices', () => {
    render(
      <DynamicPricingBreakdown
        billingExpr='tier("base", p * 8 + c * 16)'
        currencyUSDFactor={0.125}
      />
    )
    expect(screen.getAllByText('$1.0000').length).toBeGreaterThan(0)
    expect(screen.getAllByText('$2.0000').length).toBeGreaterThan(0)
    expect(screen.queryByText('$8.0000')).not.toBeInTheDocument()
  })
})
