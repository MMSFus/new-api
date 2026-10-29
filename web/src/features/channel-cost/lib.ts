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
import { formatLogQuota } from '@/lib/format'

import type { ChannelCostAmount } from './api'

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
  return formatLogQuota(Math.round(value))
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
