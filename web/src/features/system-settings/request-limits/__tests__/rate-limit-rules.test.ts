import { describe, expect, it } from 'vitest'

import {
  migrateLegacyRateLimits,
  orderRateLimitOptionUpdates,
  remainingLegacyRateLimits,
} from '../rate-limit-rules'

describe('legacy request limit migration', () => {
  it('keeps auto rules in the legacy table while migrating concrete groups', () => {
    const legacy = '{"auto":[0,1],"vip":[0,2]}'

    expect(migrateLegacyRateLimits(legacy, '{}')).toContain('"vip"')
    expect(migrateLegacyRateLimits(legacy, '{}')).not.toContain('"auto"')
    expect(remainingLegacyRateLimits(legacy)).toBe(
      JSON.stringify({ auto: [0, 1] }, null, 2)
    )
  })

  it('writes replacement rules before clearing migrated legacy rules', () => {
    const updates: Array<[string, unknown]> = [
      ['ModelRequestRateLimitGroup', '{}'],
      ['ModelRequestRateLimitGlobalGroup', '{"vip":{"total":0,"success":2}}'],
      ['ModelRequestRateLimitEnabled', true],
    ]

    expect(orderRateLimitOptionUpdates(updates).map(([key]) => key)).toEqual([
      'ModelRequestRateLimitGlobalGroup',
      'ModelRequestRateLimitEnabled',
      'ModelRequestRateLimitGroup',
    ])
  })
})
