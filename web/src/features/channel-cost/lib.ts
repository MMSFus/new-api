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
import type { TFunction } from 'i18next'
import z from 'zod'

import dayjs from '@/lib/dayjs'
import { formatLogQuota } from '@/lib/format'
import { logRangeSearchSchema } from '@/lib/log-range-search'

import type {
  ChannelCostAmount,
  ChannelCostBucket,
  ChannelCostFilters,
  ChannelCostReport,
  ChannelCostSnapshot,
} from './api'

/** Profit over the revenue whose cost is known; null when none is. */
export function profitMargin(amount: ChannelCostAmount): number | null {
  if (amount.costed_revenue === 0) return null
  return amount.profit / amount.costed_revenue
}

export function formatPercent(
  value: number | null,
  locale: Intl.LocalesArgument
): string {
  if (value === null) return '-'
  return new Intl.NumberFormat(locale, {
    style: 'percent',
    maximumFractionDigits: 1,
  }).format(value)
}

/** Cost and profit are fractional quota; show them like charged quota. */
export function formatCostQuota(value: number): string {
  return formatLogQuota(value)
}

/** True when the snapshot converted between the upstream and price currencies. */
export function hasCurrencyFactor(cost: ChannelCostSnapshot): boolean {
  return typeof cost.currency_factor === 'number' && cost.currency_factor !== 1
}

/** "Official price X × ratio", with "× factor" when currencies differ. */
export function formatCostFormula(
  t: TFunction,
  cost: ChannelCostSnapshot
): string {
  const formula = t('Official price {{base}} × {{ratio}}', {
    base: formatCostQuota(cost.base_quota),
    ratio: cost.upstream_ratio,
  })
  return hasCurrencyFactor(cost)
    ? `${formula} × ${cost.currency_factor}`
    : formula
}

export function profitTone(value: number): string | undefined {
  if (value < 0) return 'text-destructive'
  if (value > 0) return 'text-success'
  return undefined
}

/** Adds two amounts; used to fold hourly buckets into days. */
export function addAmounts(
  a: ChannelCostAmount,
  b: ChannelCostAmount
): ChannelCostAmount {
  return {
    requests: a.requests + b.requests,
    refunds: a.refunds + b.refunds,
    uncosted_requests: a.uncosted_requests + b.uncosted_requests,
    revenue: a.revenue + b.revenue,
    costed_revenue: a.costed_revenue + b.costed_revenue,
    cost: a.cost + b.cost,
    profit: a.profit + b.profit,
    prompt_tokens: a.prompt_tokens + b.prompt_tokens,
    completion_tokens: a.completion_tokens + b.completion_tokens,
    simulated_cache_delta: a.simulated_cache_delta + b.simulated_cache_delta,
  }
}

export const CHANNEL_COST_DIMENSIONS = [
  'channel',
  'model',
  'group',
  'user',
] as const
export type ChannelCostDimension = (typeof CHANNEL_COST_DIMENSIONS)[number]

/** Route search: the shared log range plus the breakdown dimension shown. */
export const channelCostSearchSchema = logRangeSearchSchema.extend({
  dimension: z.enum(CHANNEL_COST_DIMENSIONS).optional().catch(undefined),
})

/**
 * A slice of the report opened in the drill sheet: its totals come with the
 * report, its logs are queried with `patch` merged into the page filters.
 */
export interface ChannelCostScope {
  title: string
  patch: Partial<ChannelCostFilters>
  amount: ChannelCostAmount
  /** Channel scopes show the channel's current upstream ratio. */
  upstreamRatio?: number | null
}

export interface BreakdownRow {
  key: string
  label: string
  /** Extra column: the channel's current upstream ratio. */
  ratio?: number | null
  amount: ChannelCostAmount
  /** Filter selecting this row; null when the key cannot be filtered on. */
  patch: Partial<ChannelCostFilters> | null
}

export function breakdownRows(
  report: ChannelCostReport | undefined,
  dimension: ChannelCostDimension
): BreakdownRow[] {
  if (!report) return []
  switch (dimension) {
    case 'channel':
      return (report.channels ?? []).map((row) => ({
        key: String(row.channel_id),
        label: [row.channel_name, `#${row.channel_id}`]
          .filter(Boolean)
          .join(' '),
        ratio: row.upstream_ratio,
        amount: row,
        patch: row.channel_id > 0 ? { channel: row.channel_id } : null,
      }))
    case 'model':
      return (report.models ?? []).map((row) => ({
        key: row.model_name,
        label: row.model_name || '-',
        amount: row,
        patch: row.model_name ? { model_name: row.model_name } : null,
      }))
    case 'group':
      return (report.groups ?? []).map((row) => ({
        key: row.group,
        label: row.group || '-',
        amount: row,
        patch: row.group ? { group: row.group } : null,
      }))
    case 'user':
      return (report.users ?? []).map((row) => ({
        key: String(row.user_id),
        label: [row.username, `#${row.user_id}`].filter(Boolean).join(' '),
        amount: row,
        patch: row.user_id > 0 ? { user_id: row.user_id } : null,
      }))
  }
}

/** Ranges up to this long plot hours; longer ones fold hours into days. */
const HOURLY_RANGE_SECONDS = 2 * 24 * 3600

export interface TrendPoint {
  label: string
  /** Stable chart identity; hourly values are UTC bucket timestamps. */
  key: string
  /** Inclusive bounds of the bucket, clamped to the searched range. */
  start: number
  end: number
  amount: ChannelCostAmount
}

/** Groups hourly buckets into viewer-local days when the range is long. */
export function trendPoints(
  trend: ChannelCostBucket[],
  range: { start: number; end: number }
): TrendPoint[] {
  const hourly = range.end - range.start <= HOURLY_RANGE_SECONDS
  const format = hourly ? 'MM-DD HH:mm Z' : 'YYYY-MM-DD'
  const points = new Map<string, TrendPoint>()
  for (const bucket of trend) {
    const time = dayjs.unix(bucket.time)
    const label = time.format(format)
    const key = hourly ? String(bucket.time) : label
    const previous = points.get(key)
    if (previous) {
      previous.amount = addAmounts(previous.amount, bucket)
      previous.start = Math.min(previous.start, bucket.time)
      previous.end = Math.max(previous.end, Math.min(range.end, bucket.time + 3599))
      continue
    }
    points.set(key, {
      label,
      key,
      start: Math.max(range.start, bucket.time),
      end: Math.min(range.end, bucket.time + 3599),
      amount: bucket,
    })
  }
  return [...points.values()]
}

/** Start and end of a backfill range in the viewer's time zone. */
export function formatBackfillRange(start?: number, end?: number): string {
  return `${dayjs.unix(start ?? 0).format('YYYY-MM-DD HH:mm')} — ${dayjs.unix(end ?? 0).format('YYYY-MM-DD HH:mm')}`
}
