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
import { t } from 'i18next'

import type { ApiResponse } from '@/features/profile/types'
import type { UsageLog } from '@/features/usage-logs/data/schema'
import { api } from '@/lib/api'
import { createServerError } from '@/lib/server-error-message'

/** Shared by the list and stat endpoints; omitted keys use backend defaults. */
export interface SimulatedCacheFilters {
  start_timestamp?: number
  end_timestamp?: number
  channel?: number
  model_name?: string
  username?: string
}

export interface SimulatedCacheStat {
  count: number
  original_quota: number
  simulated_quota: number
  quota_delta: number
  original_cache_read_tokens: number
  simulated_cache_read_tokens: number
  simulated_cache_creation_tokens: number
}

export interface SimulatedCacheLogPage {
  items: UsageLog[]
  total: number
}

/** Backend rejects ranges longer than this. */
export const SIMULATED_CACHE_MAX_RANGE_SECONDS = 7 * 24 * 3600

export async function getSimulatedCacheLogs(
  params: SimulatedCacheFilters & { p: number; page_size: number }
): Promise<SimulatedCacheLogPage> {
  const response = await api.get<ApiResponse<SimulatedCacheLogPage>>(
    '/api/log/simulated_cache',
    { params }
  )
  if (!response.data.success || !response.data.data) {
    throw createServerError(
      response.data,
      t('Failed to load simulated cache logs')
    )
  }
  return response.data.data
}

export async function getSimulatedCacheStat(
  params: SimulatedCacheFilters
): Promise<SimulatedCacheStat> {
  const response = await api.get<ApiResponse<SimulatedCacheStat>>(
    '/api/log/simulated_cache/stat',
    { params }
  )
  if (!response.data.success || !response.data.data) {
    throw createServerError(
      response.data,
      t('Failed to load simulated cache statistics')
    )
  }
  return response.data.data
}
