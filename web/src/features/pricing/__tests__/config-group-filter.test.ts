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

import { FILTER_ALL } from '../constants'
import { filterByGroup, isModelInConfigGroup } from '../lib/filters'
import type { PricingConfigGroup, PricingModel } from '../types'

function pricingModel(name: string, groups: string[]): PricingModel {
  return {
    id: 1,
    model_name: name,
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: groups,
  }
}

const models = [
  pricingModel('default-only', ['default']),
  pricingModel('vip-only', ['vip']),
  pricingModel('svip-only', ['svip']),
]

const premium: PricingConfigGroup = {
  key: 'premium',
  value: 'cfg:premium',
  name: 'Premium',
  groups: ['svip', 'vip'],
}

describe('config group filtering', () => {
  it('keeps models enabled in any member group when a config group is selected', () => {
    const result = filterByGroup(models, 'cfg:premium', [premium])

    expect(result.map((m) => m.model_name)).toEqual(['vip-only', 'svip-only'])
  })

  it('keeps filtering by a plain group when the value is not a config group', () => {
    const result = filterByGroup(models, 'vip', [premium])

    expect(result.map((m) => m.model_name)).toEqual(['vip-only'])
  })

  it('returns no models for an unknown config group reference', () => {
    expect(filterByGroup(models, 'cfg:missing', [premium])).toEqual([])
  })

  it('returns all models when no group is selected', () => {
    expect(filterByGroup(models, FILTER_ALL, [premium])).toBe(models)
  })

  it('treats a model as unreachable when no member group enables it', () => {
    expect(isModelInConfigGroup(models[0], premium)).toBe(false)
    expect(isModelInConfigGroup(models[0], { ...premium, groups: [] })).toBe(
      false
    )
  })
})
