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

import type { LogOtherData, SimulatedCacheUsage } from '../types'
import { computeCacheHitRatio } from './cache-hit-ratio'

export interface SimulatedCacheSide {
  promptTokens: number
  completionTokens: number
  cacheReadTokens: number
  cacheCreationTokens: number
  cacheCreation5mTokens: number
  cacheCreation1hTokens: number
  /** null when the backend did not record a quota for this side. */
  quota: number | null
}

export interface SimulatedCacheAudit {
  version: number | null
  hitRate: number | null
  hitRateMin: number | null
  hitRateMax: number | null
  /** Rate drawn for this request, percent; null on a miss or older rows. */
  targetRate: number | null
  clientFormat: string
  upstreamSchema: string
  matchedSegments: number | null
  totalSegments: number | null
  original: SimulatedCacheSide
  simulated: SimulatedCacheSide
  quotaDelta: number | null
}

function finiteOrNull(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

function readSide(raw: unknown): SimulatedCacheSide {
  const usage: SimulatedCacheUsage =
    raw !== null && typeof raw === 'object' ? (raw as SimulatedCacheUsage) : {}
  const creation5m = finiteOrNull(usage.cache_creation_5m_tokens) ?? 0
  const creation1h = finiteOrNull(usage.cache_creation_1h_tokens) ?? 0
  return {
    promptTokens: finiteOrNull(usage.prompt_tokens) ?? 0,
    completionTokens: finiteOrNull(usage.completion_tokens) ?? 0,
    cacheReadTokens: finiteOrNull(usage.cache_read_tokens) ?? 0,
    cacheCreationTokens:
      finiteOrNull(usage.cache_creation_tokens) ?? creation5m + creation1h,
    cacheCreation5mTokens: creation5m,
    cacheCreation1hTokens: creation1h,
    quota: finiteOrNull(usage.quota),
  }
}

/**
 * Reads `other.admin_info.simulated_cache`. The object is versioned and only
 * ever gains keys, so unknown keys are ignored and missing numbers fall back
 * to 0 (tokens) or null (quota, rates, segments). Returns null when the log
 * carries no simulated cache marker.
 */
export function getSimulatedCache(
  other: LogOtherData | null | undefined
): SimulatedCacheAudit | null {
  const raw: unknown = other?.admin_info?.simulated_cache
  if (raw === null || typeof raw !== 'object' || Array.isArray(raw)) {
    return null
  }
  const info = raw as Record<string, unknown>
  const original = readSide(info.original)
  const simulated = readSide(info.simulated)
  let quotaDelta = finiteOrNull(info.quota_delta)
  if (
    quotaDelta === null &&
    original.quota !== null &&
    simulated.quota !== null
  ) {
    quotaDelta = simulated.quota - original.quota
  }
  return {
    version: finiteOrNull(info.version),
    hitRate: finiteOrNull(info.hit_rate),
    hitRateMin: finiteOrNull(info.hit_rate_min),
    hitRateMax: finiteOrNull(info.hit_rate_max),
    targetRate: finiteOrNull(info.target_rate),
    clientFormat:
      typeof info.client_format === 'string' ? info.client_format : '',
    upstreamSchema:
      typeof info.upstream_schema === 'string' ? info.upstream_schema : '',
    matchedSegments: finiteOrNull(info.matched_segments),
    totalSegments: finiteOrNull(info.total_segments),
    original,
    simulated,
    quotaDelta,
  }
}

/**
 * Share of input served from cache, in percent (null without prompt). Audit
 * prompt_tokens is already the total input for every format, so this is the
 * same ratio the log list shows.
 */
export function getCacheHitRatio(side: SimulatedCacheSide): number | null {
  return computeCacheHitRatio(side.cacheReadTokens, side.promptTokens)
}

/** Signed quota change; positive means the simulated charge is higher. */
export function formatQuotaDelta(delta: number | null): string {
  if (delta === null) return '-'
  if (delta > 0) return `+${formatLogQuota(delta)}`
  return formatLogQuota(delta)
}
