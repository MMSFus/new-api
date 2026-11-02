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
import { getRouteApi } from '@tanstack/react-router'

import { useLogRangeSearch } from '@/hooks/use-log-range-search'

import type { SessionFilters } from './api'
import type { RevenueBreakdown, SessionRecorderTab } from './lib'

const route = getRouteApi('/_authenticated/session-recorder/')

export interface SessionFilterPatch {
  user_id?: number
  session_id?: string
}

/**
 * URL state of the page. The range and model filter are shared by both tabs;
 * user and session only filter the session list. Tab changes keep filters
 * and return to the first page.
 */
export function useSessionRecorderSearch() {
  const routeSearch = route.useSearch()
  const navigate = route.useNavigate()
  const range = useLogRangeSearch({
    search: routeSearch,
    navigate,
    defaultPreset: '7d',
  })
  const sessionFilters: SessionFilters = {
    start_timestamp: range.filters.start_timestamp,
    end_timestamp: range.filters.end_timestamp,
    model: range.filters.model_name,
    user_id: routeSearch.user,
    session_id: routeSearch.session,
  }

  const updateSessionFilters = (patch: SessionFilterPatch) => {
    navigate({
      replace: true,
      search: (prev) => ({
        ...prev,
        ...('user_id' in patch ? { user: patch.user_id } : {}),
        ...('session_id' in patch
          ? { session: patch.session_id || undefined }
          : {}),
        page: undefined,
      }),
    })
  }

  const selectTab = (tab: SessionRecorderTab, model?: string) => {
    navigate({
      search: (prev) => ({
        ...prev,
        tab,
        ...(model === undefined ? {} : { model }),
        page: undefined,
      }),
    })
  }

  const selectBreakdown = (breakdown: RevenueBreakdown) => {
    navigate({
      replace: true,
      search: (prev) => ({
        ...prev,
        breakdown: breakdown === 'rule' ? undefined : breakdown,
      }),
    })
  }

  /** Clears every filter but stays on the current tab. */
  const reset = () => {
    range.refreshNow()
    navigate({ search: (prev) => ({ tab: prev.tab }) })
  }

  return {
    routeSearch,
    range,
    sessionFilters,
    updateSessionFilters,
    selectTab,
    selectBreakdown,
    reset,
  }
}

export type SessionRecorderSearchState = ReturnType<
  typeof useSessionRecorderSearch
>
