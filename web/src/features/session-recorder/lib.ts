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
import z from 'zod'

import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import dayjs from '@/lib/dayjs'
import { formatNumber } from '@/lib/format'
import { logRangeSearchSchema } from '@/lib/log-range-search'
import { ROLE } from '@/lib/roles'
import type { AuthUser } from '@/stores/auth-store'

import type { RevenueBucket, RevenueGranularity } from './api'

export const SESSION_RECORDER_TABS = ['revenue', 'content'] as const
export type SessionRecorderTab = (typeof SESSION_RECORDER_TABS)[number]

export const REVENUE_BREAKDOWNS = ['rule', 'model'] as const
export type RevenueBreakdown = (typeof REVENUE_BREAKDOWNS)[number]

/**
 * Route search: the shared log range and model filter, plus the tab, the
 * revenue breakdown and the session filters of the content tab.
 */
export const sessionRecorderSearchSchema = logRangeSearchSchema.extend({
  tab: z.enum(SESSION_RECORDER_TABS).optional().catch(undefined),
  breakdown: z.enum(REVENUE_BREAKDOWNS).optional().catch(undefined),
  user: z.coerce.number().int().positive().optional().catch(undefined),
  session: z.string().min(1).optional().catch(undefined),
})
export type SessionRecorderSearch = z.infer<typeof sessionRecorderSearchSchema>

/** 1 TB is 10^12 bytes of compressed (archived) data. */
export const BYTES_PER_TB = 1e12

/** The revenue summary rejects longer ranges. */
export const REVENUE_MAX_RANGE_DAYS = 92

/** Ranges up to this long are summarised by hour, longer ones by day. */
const HOURLY_RANGE_SECONDS = 2 * 24 * 3600

export interface SessionRecorderAccess {
  revenue: boolean
  revenueWrite: boolean
  content: boolean
  /** Only root may run the historical index backfill. */
  backfill: boolean
}

export function sessionRecorderAccess(
  user: AuthUser | null | undefined
): SessionRecorderAccess {
  const { READ, WRITE } = ADMIN_PERMISSION_ACTIONS
  const revenue = ADMIN_PERMISSION_RESOURCES.SESSION_RECORDER_REVENUE
  const content = ADMIN_PERMISSION_RESOURCES.SESSION_RECORDER_CONTENT
  return {
    revenue: hasPermission(user, revenue, READ),
    revenueWrite:
      hasPermission(user, revenue, READ) && hasPermission(user, revenue, WRITE),
    content: hasPermission(user, content, READ),
    backfill: user?.role === ROLE.SUPER_ADMIN,
  }
}

/** Tabs the user may open, in display order. */
export function allowedTabs(
  access: SessionRecorderAccess
): SessionRecorderTab[] {
  return SESSION_RECORDER_TABS.filter((tab) => access[tab])
}

export function revenueGranularity(range: {
  start: number
  end: number
}): RevenueGranularity {
  return range.end - range.start <= HOURLY_RANGE_SECONDS ? 'hour' : 'day'
}

export interface RevenueTrendPoint {
  label: string
  /** Inclusive bounds of the bucket, clamped to the searched range. */
  start: number
  end: number
  bucket: RevenueBucket
}

/**
 * One point per bucket of the range, zero where the server returned none.
 * The server aligns buckets to UTC hours or UTC midnight, so the zero
 * buckets are aligned the same way; day labels are UTC dates for the same
 * reason, hour labels local time.
 */
export function revenueTrendPoints(
  trend: RevenueBucket[],
  range: { start: number; end: number }
): RevenueTrendPoint[] {
  const unit = revenueGranularity(range)
  const step = unit === 'hour' ? 3600 : 86400
  const byBucket = new Map(trend.map((bucket) => [bucket.bucket, bucket]))
  const points: RevenueTrendPoint[] = []
  if (range.end < range.start) return points
  for (
    let time = Math.floor(range.start / step) * step;
    time <= range.end;
    time += step
  ) {
    const bucket = byBucket.get(time) ?? {
      bucket: time,
      archive_bytes: 0,
      revenue: 0,
    }
    points.push({
      label:
        unit === 'hour'
          ? dayjs.unix(time).format('MM-DD HH:00')
          : new Date(time * 1000).toISOString().slice(0, 10),
      start: Math.max(range.start, time),
      end: Math.min(range.end, time + step - 1),
      bucket,
    })
  }
  return points
}

/**
 * Compressed bytes in TB. Tiny amounts keep three significant digits so a
 * non-zero archive never reads as 0.
 */
export function formatTerabytes(
  bytes: number,
  locale: Intl.LocalesArgument
): string {
  const value = bytes / BYTES_PER_TB
  if (value > 0 && value < 0.01) {
    // formatNumber caps at two fraction digits, which would round this to 0.
    return new Intl.NumberFormat(locale, {
      maximumSignificantDigits: 3,
    }).format(value)
  }
  return formatNumber(value, locale)
}

const DECIMAL_BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'] as const

/** Byte size with decimal (SI) units, matching the 10^12 bytes TB. */
export function formatDecimalBytes(
  bytes: number,
  locale: Intl.LocalesArgument
): string {
  let value = bytes
  let unit = 0
  while (Math.abs(value) >= 1000 && unit < DECIMAL_BYTE_UNITS.length - 1) {
    value /= 1000
    unit++
  }
  return `${formatNumber(value, locale)} ${DECIMAL_BYTE_UNITS[unit]}`
}

export function formatUnixTime(seconds: number): string {
  if (!seconds) return '-'
  return dayjs.unix(seconds).format('YYYY-MM-DD HH:mm:ss')
}
