import { describe, expect, it } from 'vitest'

import type { ChannelCostAmount } from '../api'
import { formatCostQuota, trendPoints } from '../lib'

const amount: ChannelCostAmount = {
  requests: 0,
  refunds: 0,
  uncosted_requests: 0,
  revenue: 0,
  costed_revenue: 0,
  cost: 0,
  profit: 0,
  prompt_tokens: 0,
  completion_tokens: 0,
  simulated_cache_delta: 0,
}

describe('channel cost display', () => {
  it('preserves fractional and negative quota amounts', () => {
    expect(formatCostQuota(0.3)).not.toBe(formatCostQuota(0))
    expect(formatCostQuota(-0.3)).not.toBe(formatCostQuota(0))
  })

  it('keeps a non-hour local zone aligned to the original UTC bucket', () => {
    const previousTimeZone = process.env.TZ
    process.env.TZ = 'Asia/Kolkata'
    try {
      const [point] = trendPoints([{ time: 1_793_491_200, ...amount }], {
        start: 1_793_491_200,
        end: 1_793_494_799,
      })

      expect(point.start).toBe(1_793_491_200)
      expect(point.end).toBe(1_793_494_799)
      expect(point.label).toBe('11-01 05:30 +05:30')
    } finally {
      if (previousTimeZone === undefined) delete process.env.TZ
      else process.env.TZ = previousTimeZone
    }
  })

  it('uses UTC hour bucket bounds and distinct local labels for repeated hours', () => {
    const previousTimeZone = process.env.TZ
    process.env.TZ = 'America/New_York'
    try {
      const buckets = trendPoints(
        [
          { time: 1_793_509_200, ...amount },
          { time: 1_793_512_800, ...amount },
        ],
        { start: 1_793_509_200, end: 1_793_516_399 }
      )

      expect(buckets).toHaveLength(2)
      expect(buckets[0].start).toBe(1_793_509_200)
      expect(buckets[0].end).toBe(1_793_512_799)
      expect(buckets[0].label).not.toBe(buckets[1].label)
    } finally {
      if (previousTimeZone === undefined) delete process.env.TZ
      else process.env.TZ = previousTimeZone
    }
  })

  it('drills long-range local-day totals across every included UTC bucket', () => {
    const first = 1_793_491_200
    const last = first + 3 * 3600
    const points = trendPoints(
      [
        { time: first, ...amount },
        { time: last, ...amount },
      ],
      { start: first, end: last + 3599 + 48 * 3600 }
    )

    expect(points[0].start).toBe(first)
    expect(points[0].end).toBe(last + 3599)
  })
})
