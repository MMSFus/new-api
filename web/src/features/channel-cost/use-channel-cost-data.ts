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
import { useTranslation } from 'react-i18next'

import { requireServerSuccess } from '@/lib/server-error-message'

import {
  CHANNEL_COST_MAX_RANGE_SECONDS,
  getChannelCostLogs,
  getChannelCostReport,
  type ChannelCostFilters,
  type ChannelCostLog,
} from './api'

const EMPTY_LOGS: ChannelCostLog[] = []

/** Client-side copy of the backend range rule; '' when the range is valid. */
export function useChannelCostRangeError(filters: ChannelCostFilters): string {
  const { t } = useTranslation()
  const end = filters.end_timestamp ?? 0
  const start = filters.start_timestamp ?? 0
  if (start > end) return t('End time must be after start time')
  if (end - start > CHANNEL_COST_MAX_RANGE_SECONDS) {
    return t('Time range cannot exceed {{days}} days', {
      days: CHANNEL_COST_MAX_RANGE_SECONDS / (24 * 3600),
    })
  }
  return ''
}

export function useChannelCostReport(
  filters: ChannelCostFilters,
  enabled: boolean
) {
  return useQuery({
    queryKey: ['channel-cost', 'report', filters],
    queryFn: async () =>
      requireServerSuccess(await getChannelCostReport(filters)),
    enabled,
    retry: false,
  })
}

/** One page of cost logs; shared by the page table and the drill sheet. */
export function useChannelCostLogs(
  filters: ChannelCostFilters,
  pagination: PaginationState,
  enabled: boolean
) {
  const params = {
    ...filters,
    p: pagination.pageIndex + 1,
    page_size: pagination.pageSize,
  }
  const query = useQuery({
    queryKey: ['channel-cost', 'logs', params],
    queryFn: async () => requireServerSuccess(await getChannelCostLogs(params)),
    enabled,
    retry: false,
  })
  return {
    query,
    logs: enabled && query.data ? query.data.items : EMPTY_LOGS,
    total: enabled && !query.isError ? (query.data?.total ?? 0) : 0,
  }
}
