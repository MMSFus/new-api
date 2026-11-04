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

import {
  buildPricingChanges,
  type ModelPricingConfig,
} from '@/features/model-pricing/api'
import {
  applyPriceSyncSelections,
  pricingOptions,
} from '@/features/model-pricing/pricing'

// "cny-model" is priced in CNY; "other" keeps its own CNY mark.
const before = pricingOptions({
  ModelRatio: JSON.stringify({ 'cny-model': 3.65, other: 1 }),
  ModelPriceCurrency: JSON.stringify({ 'cny-model': 'CNY', other: 'CNY' }),
  'billing_setting.billing_mode': JSON.stringify({ 'cny-model': 'ratio' }),
})
const snapshot: ModelPricingConfig = {
  entries: [
    {
      model_name: 'cny-model',
      version: 'v1',
      configured: {
        ModelRatio: 3.65,
        ModelPriceCurrency: 'CNY',
        'billing_setting.billing_mode': 'ratio',
      },
      effective: {
        ModelRatio: 3.65,
        ModelPriceCurrency: 'CNY',
        'billing_setting.billing_mode': 'ratio',
      },
    },
  ],
  options: before,
  empty_version: 'empty',
}

describe('upstream price sync with a currency mark', () => {
  it('saves a currency-only change when the USD expression text is identical', () => {
    const expression = 'tier("base", p * 7)'
    const configured = {
      ModelPriceCurrency: 'CNY',
      'billing_setting.billing_mode': 'tiered_expr',
      'billing_setting.billing_expr': expression,
    }
    const expressionBefore = pricingOptions({
      ...before,
      'billing_setting.billing_mode': JSON.stringify({
        'cny-model': 'tiered_expr',
      }),
      'billing_setting.billing_expr': JSON.stringify({
        'cny-model': expression,
      }),
    })
    const expressionSnapshot: ModelPricingConfig = {
      ...snapshot,
      options: expressionBefore,
      entries: [{ ...snapshot.entries[0], configured, effective: configured }],
    }
    const after = applyPriceSyncSelections(expressionBefore, {
      'cny-model': { billing_mode: 'tiered_expr', billing_expr: expression },
    })
    expect(
      buildPricingChanges(expressionSnapshot, expressionBefore, after)
    ).toEqual([
      {
        model_name: 'cny-model',
        expected_version: 'v1',
        pricing: {
          'billing_setting.billing_mode': 'tiered_expr',
          'billing_setting.billing_expr': expression,
        },
      },
    ])
    expect(JSON.parse(after.ModelPriceCurrency)).toEqual({ other: 'CNY' })
  })

  it.each([
    ['a USD ratio', { model_ratio: 0.5, billing_mode: 'ratio' }],
    ['a USD fixed price', { model_price: 0.02, billing_mode: 'ratio' }],
    [
      'a USD expression',
      {
        billing_mode: 'tiered_expr',
        billing_expr: 'tier("base", p * 1 + c * 2)',
      },
    ],
  ])(
    'overwriting with %s removes the model mark from the PATCH draft',
    (_label, upstream) => {
      const after = applyPriceSyncSelections(before, { 'cny-model': upstream })
      const [change] = buildPricingChanges(snapshot, before, after)
      expect(change.model_name).toBe('cny-model')
      expect(change.pricing).not.toHaveProperty('ModelPriceCurrency')
    }
  )

  it('keeps the marks of models that were not synchronized', () => {
    const after = applyPriceSyncSelections(before, {
      'cny-model': { model_ratio: 0.5, billing_mode: 'ratio' },
    })
    expect(JSON.parse(after.ModelPriceCurrency)).toEqual({ other: 'CNY' })
  })

  it('never takes a currency from upstream fields', () => {
    const after = applyPriceSyncSelections(before, {
      'cny-model': {
        model_ratio: 0.5,
        model_price_currency: 'CUSTOM',
        billing_mode: 'ratio',
      },
    })
    expect(JSON.parse(after.ModelPriceCurrency)).not.toHaveProperty('cny-model')
  })
})
