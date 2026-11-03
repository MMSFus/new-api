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
import { describe, expect, test } from 'vitest'

import { CHANNEL_TYPE_SUB2API } from '../../constants'
import { channelSchema } from '../../types'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  buildSettingJSON,
  channelFormSchema,
  transformChannelToFormDefaults,
  type ChannelFormValues,
} from '../channel-form'

function sub2apiForm(overrides: Partial<ChannelFormValues> = {}) {
  return {
    ...CHANNEL_FORM_DEFAULT_VALUES,
    name: 'Sub2API upstream',
    type: CHANNEL_TYPE_SUB2API,
    base_url: 'https://sub2api.example',
    key: 'sk-test',
    models: 'claude-sonnet-5-5',
    upstream_cost_ratio: '0.2',
    ...overrides,
  }
}

function issuePaths(values: ChannelFormValues): string[] {
  const result = channelFormSchema.safeParse(values)
  return result.success
    ? []
    : result.error.issues.map((issue) => issue.path.join('.'))
}

describe('upstream cost ratio sync settings', () => {
  test('saving a Sub2API channel with sync and a range writes all sync fields', () => {
    const setting = JSON.parse(
      buildSettingJSON(
        sub2apiForm({
          upstream_cost_currency: 'USD',
          upstream_cost_ratio_sync_enabled: true,
          upstream_cost_ratio_sync_min: '0.1',
          upstream_cost_ratio_sync_max: '0.5',
        })
      )
    )

    expect(setting).toMatchObject({
      upstream_cost_ratio: 0.2,
      upstream_cost_currency: 'USD',
      upstream_cost_ratio_sync_enabled: true,
      upstream_cost_ratio_sync_min: 0.1,
      upstream_cost_ratio_sync_max: 0.5,
    })
  })

  test('the site currency and empty range bounds are omitted on save', () => {
    const setting = JSON.parse(
      buildSettingJSON(sub2apiForm({ upstream_cost_ratio_sync_enabled: true }))
    )

    expect(setting).not.toHaveProperty('upstream_cost_currency')
    expect(setting).not.toHaveProperty('upstream_cost_ratio_sync_min')
    expect(setting).not.toHaveProperty('upstream_cost_ratio_sync_max')
    expect(setting.upstream_cost_ratio_sync_enabled).toBe(true)
  })

  test('an OpenAI channel on a custom upstream saves the sync switch', () => {
    const setting = JSON.parse(
      buildSettingJSON(
        sub2apiForm({ type: 1, upstream_cost_ratio_sync_enabled: true })
      )
    )

    expect(setting.upstream_cost_ratio_sync_enabled).toBe(true)
  })

  test('enabling sync on an official API host is rejected on the switch', () => {
    expect(
      issuePaths(
        sub2apiForm({
          type: 14,
          base_url: 'https://api.anthropic.com',
          upstream_cost_ratio_sync_enabled: true,
        })
      )
    ).toEqual(['upstream_cost_ratio_sync_enabled'])
  })

  test('saved sync settings load back into the form as text inputs', () => {
    const channel = channelSchema.parse({
      id: 7,
      name: 'Sub2API upstream',
      type: CHANNEL_TYPE_SUB2API,
      key: '',
      status: 1,
      created_time: 1,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
      models: 'claude-sonnet-5-5',
      group: 'default',
      setting: JSON.stringify({
        upstream_cost_ratio: 0.2,
        upstream_cost_currency: 'CUSTOM',
        upstream_cost_ratio_sync_enabled: true,
        upstream_cost_ratio_sync_max: 0.5,
      }),
    })

    const form = transformChannelToFormDefaults(channel)

    expect(form).toMatchObject({
      upstream_cost_currency: 'CUSTOM',
      upstream_cost_ratio_sync_enabled: true,
      upstream_cost_ratio_sync_min: '',
      upstream_cost_ratio_sync_max: '0.5',
    })
  })

  test('an unknown saved currency falls back to the site currency', () => {
    const channel = channelSchema.parse({
      id: 8,
      name: 'Sub2API upstream',
      type: CHANNEL_TYPE_SUB2API,
      key: '',
      status: 1,
      created_time: 1,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
      models: 'claude-sonnet-5-5',
      group: 'default',
      setting: JSON.stringify({ upstream_cost_currency: 'EUR' }),
    })

    expect(transformChannelToFormDefaults(channel).upstream_cost_currency).toBe(
      ''
    )
  })

  test('a range minimum above the maximum is rejected on the maximum field', () => {
    expect(
      issuePaths(
        sub2apiForm({
          upstream_cost_ratio_sync_enabled: true,
          upstream_cost_ratio_sync_min: '0.6',
          upstream_cost_ratio_sync_max: '0.5',
        })
      )
    ).toEqual(['upstream_cost_ratio_sync_max'])
  })

  test('a range bound outside 0-100 is rejected on that field', () => {
    expect(
      issuePaths(
        sub2apiForm({
          upstream_cost_ratio_sync_enabled: true,
          upstream_cost_ratio_sync_min: '-1',
        })
      )
    ).toEqual(['upstream_cost_ratio_sync_min'])
  })

  test('range bounds are not validated while sync is off', () => {
    expect(
      issuePaths(sub2apiForm({ upstream_cost_ratio_sync_min: '-1' }))
    ).toEqual([])
  })
})
