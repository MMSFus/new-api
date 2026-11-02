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
import { useQuery } from '@tanstack/react-query'
import type { PaginationState } from '@tanstack/react-table'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { parseLogOther } from '@/features/usage-logs/lib/format'
import { getSimulatedCache } from '@/features/usage-logs/lib/simulated-cache'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  getSimulatedCacheLogs,
  getSimulatedCacheStat,
  SIMULATED_CACHE_MAX_RANGE_SECONDS,
  type SimulatedCacheFilters,
} from './api'
import type { SimulatedCacheRow } from './types'

const EMPTY_ROWS: SimulatedCacheRow[] = []

/** Client-side copy of the backend range rule; '' when the range is valid. */
export function useSimulatedCacheRangeError(
  filters: SimulatedCacheFilters
): string {
  const { t } = useTranslation()
  const end = filters.end_timestamp ?? 0
  const start = filters.start_timestamp ?? 0
  if (start > end) return t('End time must be after start time')
  if (end - start > SIMULATED_CACHE_MAX_RANGE_SECONDS) {
    return t('Time range cannot exceed 7 days')
  }
  return ''
}

/** Stat and one page of audit rows for a search; used by page and drill sheet. */
export function useSimulatedCacheData(
  filters: SimulatedCacheFilters,
  pagination: PaginationState,
  enabled: boolean
) {
  const statQuery = useQuery({
    queryKey: ['simulated-cache', 'stat', filters],
    queryFn: async () =>
      requireServerSuccess(await getSimulatedCacheStat(filters)),
    enabled,
    retry: false,
  })
  const listParams = {
    ...filters,
    p: pagination.pageIndex + 1,
    page_size: pagination.pageSize,
  }
  const listQuery = useQuery({
    queryKey: ['simulated-cache', 'logs', listParams],
    queryFn: async () =>
      requireServerSuccess(await getSimulatedCacheLogs(listParams)),
    enabled,
    retry: false,
  })
  const page = enabled ? listQuery.data : undefined
  const rows = useMemo(
    () =>
      page
        ? page.items.map((log) => ({
            log,
            audit: getSimulatedCache(parseLogOther(log.other)),
          }))
        : EMPTY_ROWS,
    [page]
  )
  const refetch = () => {
    if (!enabled) return
    void statQuery.refetch()
    void listQuery.refetch()
  }
  return {
    statQuery,
    listQuery,
    rows,
    total: enabled && !listQuery.isError ? (listQuery.data?.total ?? 0) : 0,
    isFetching: statQuery.isFetching || listQuery.isFetching,
    error: enabled ? (listQuery.error ?? statQuery.error) : null,
    refetch,
  }
}
