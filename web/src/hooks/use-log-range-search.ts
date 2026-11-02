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
import { useMemo, useState } from 'react'

import { useTableUrlState, type NavigateFn } from '@/hooks/use-table-url-state'
import {
  activeLogRangePreset,
  logRangeFiltersToSearch,
  resolveLogRangeFilters,
  type LogRangeFilters,
  type LogRangePreset,
  type LogRangeSearch,
} from '@/lib/log-range-search'

const nowSeconds = () => Math.floor(Date.now() / 1000)

/**
 * URL-backed filters of an admin log report. The URL is the single source of
 * truth; relative presets resolve against the last search time so the query
 * key stays stable between renders.
 */
export function useLogRangeSearch(options: {
  search: LogRangeSearch
  navigate: NavigateFn
  defaultPreset: LogRangePreset
}) {
  const [now, setNow] = useState(nowSeconds)
  const search = options.search
  const navigate = options.navigate
  const filters = useMemo(
    () => resolveLogRangeFilters(search, options.defaultPreset, now),
    [search, options.defaultPreset, now]
  )
  const table = useTableUrlState({
    search,
    navigate,
    pagination: { defaultPage: 1, defaultPageSize: 20 },
    globalFilter: { enabled: false },
  })

  /** Merges a filter patch into the URL and returns to the first page. */
  const updateFilters = (patch: Partial<LogRangeFilters>) => {
    const next = logRangeFiltersToSearch(patch)
    const rangeTouched = 'start' in next || 'end' in next
    navigate({
      replace: true,
      search: (prev) => ({
        ...prev,
        ...next,
        ...(rangeTouched ? { range: undefined } : {}),
        page: undefined,
      }),
    })
  }

  const selectPreset = (preset: LogRangePreset) => {
    setNow(nowSeconds())
    navigate({
      search: (prev) => ({
        ...prev,
        range: preset === options.defaultPreset ? undefined : preset,
        start: undefined,
        end: undefined,
        page: undefined,
      }),
    })
  }

  /**
   * Re-resolves relative bounds against the current time. Returns false when
   * the resolved filters did not change, so the caller refetches explicitly.
   */
  const refreshNow = (): boolean => {
    const next = nowSeconds()
    const changed =
      JSON.stringify(
        resolveLogRangeFilters(search, options.defaultPreset, next)
      ) !== JSON.stringify(filters)
    setNow(next)
    return changed
  }

  const reset = () => {
    setNow(nowSeconds())
    navigate({ search: {} })
  }

  return {
    filters,
    preset: activeLogRangePreset(search, options.defaultPreset),
    pagination: table.pagination,
    onPaginationChange: table.onPaginationChange,
    updateFilters,
    selectPreset,
    refreshNow,
    reset,
  }
}
