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
import { formatNumber } from '@/lib/format'

import type { LogOtherData } from '../types'

/**
 * Share of total input tokens read from cache, in percent clamped to
 * [0, 100]. Null when there is no input to divide by.
 */
export function computeCacheHitRatio(
  cacheReadTokens: number,
  totalInputTokens: number
): number | null {
  if (!(totalInputTokens > 0)) return null
  const ratio = (Math.max(0, cacheReadTokens) / totalInputTokens) * 100
  return Math.min(100, Math.max(0, ratio))
}

/**
 * Cache hit ratio of a consume log, or null when nothing was read from cache.
 * Anthropic-semantic logs report prompt_tokens without cache, so the total is
 * prompt + read + write; other logs already include cache in prompt_tokens.
 * An explicit `input_tokens_total` wins over both.
 */
export function getLogCacheHitRatio(
  promptTokens: number,
  other: LogOtherData | null | undefined
): number | null {
  const cacheRead = other?.cache_tokens || 0
  if (cacheRead <= 0) return null
  const explicitTotal = other?.input_tokens_total || 0
  if (explicitTotal > 0) return computeCacheHitRatio(cacheRead, explicitTotal)
  if (other?.usage_semantic !== 'anthropic') {
    return computeCacheHitRatio(cacheRead, promptTokens)
  }
  const cacheWrite =
    (other.cache_creation_tokens_5m || 0) +
      (other.cache_creation_tokens_1h || 0) ||
    other.cache_creation_tokens ||
    0
  return computeCacheHitRatio(cacheRead, promptTokens + cacheRead + cacheWrite)
}

/** Formats a ratio from this module as `xx.x%` (null as `-`). */
export function formatCacheHitRatio(
  ratio: number | null,
  locale?: Intl.LocalesArgument
): string {
  if (ratio === null) return '-'
  return `${formatNumber(Math.round(ratio * 10) / 10, locale)}%`
}
