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

import { CHANNEL_TYPE_KIMI } from '../../constants'
import {
  getUpstreamRatioSyncEligibility,
  UPSTREAM_RATIO_INELIGIBLE_REASONS,
} from '../upstream-ratio-sync'

const ANTHROPIC = 14
const OPENAI = 1

describe('upstream ratio sync eligibility', () => {
  test.each([
    [ANTHROPIC, 'https://supeai.top'],
    [OPENAI, 'https://relay.example.com/v1'],
    [ANTHROPIC, 'http://10.0.0.5:8080'],
  ])('a type %i channel on custom host %s is eligible', (type, baseUrl) => {
    expect(getUpstreamRatioSyncEligibility(type, baseUrl)).toEqual({
      eligible: true,
    })
  })

  test.each([
    'https://api.anthropic.com',
    'https://api.openai.com/v1',
    'https://API.OpenAI.com:443',
    'https://api.deepseek.com.',
  ])('an official API host %s is ineligible', (baseUrl) => {
    expect(getUpstreamRatioSyncEligibility(ANTHROPIC, baseUrl)).toEqual({
      eligible: false,
      reason: UPSTREAM_RATIO_INELIGIBLE_REASONS.OFFICIAL_API,
    })
  })

  test('a lookalike domain that only ends with an official name stays eligible', () => {
    expect(
      getUpstreamRatioSyncEligibility(OPENAI, 'https://notopenai.com')
    ).toEqual({ eligible: true })
  })

  test.each(['', '   ', 'relay.example.com', 'ftp://relay.example.com'])(
    'a missing or non-http base URL %j is ineligible',
    (baseUrl) => {
      expect(getUpstreamRatioSyncEligibility(OPENAI, baseUrl)).toEqual({
        eligible: false,
        reason: UPSTREAM_RATIO_INELIGIBLE_REASONS.NEEDS_CUSTOM_BASE_URL,
      })
    }
  )

  test('an OAuth credential channel is ineligible even on a custom host', () => {
    expect(
      getUpstreamRatioSyncEligibility(
        CHANNEL_TYPE_KIMI,
        'https://relay.example.com'
      )
    ).toEqual({
      eligible: false,
      reason: UPSTREAM_RATIO_INELIGIBLE_REASONS.CREDENTIALS,
    })
  })
})
